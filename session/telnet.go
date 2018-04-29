package session

import (
	"net"
	"strconv"
	"strings"
	"sync"
)

const (
	// special telnet octets for telnet commands
	IAC  = byte(255) //[I]nterpret [A]s [C]ommand
	DONT = byte(254)
	DO   = byte(253)
	WONT = byte(252)
	WILL = byte(251)
	SB   = byte(250) //[S]equence [B]egin
	SE   = byte(240) //[S]equence [E]end

	//special command for getting term size
	NAWS = byte(31) //[N]egotiate [A]bout [W]indow [S]ize

	// default buffer size for reading messages from connection
	EXPECTED_MSG_SIZE = 128
)

var (
	// default term color for messages (white)
	MESSAGE_COLOR = "\033[1;37m"
	// default term color for events (light gray)
	EVENT_COLOR = "\033[1;30m"

	// ensure Telnet adheres to the Session interface
	_ Session = (*Telnet)(nil)

	// mapping of plain text colors to terminal escape sequence colors
	TELNET_USERNAME_COLORS = map[string]string{
		"red":     "\033[0;31m",
		"orange":  "\033[1;31m",
		"green":   "\033[0;32m",
		"lime":    "\033[1;32m",
		"brown":   "\033[0;33m",
		"yellow":  "\033[1;33m",
		"blue":    "\033[0;34m",
		"indigo":  "\033[1;34m",
		"purple":  "\033[0;35m",
		"fuschia": "\033[1;35m",
		"cyan":    "\033[0;36m",
		"aqua":    "\033[1;36m",
	}

	// in the event we get a color we don't recognie, just use fuschia
	DEFAULT_TELNET_USERNAME_COLOR = TELNET_USERNAME_COLORS["fuschia"]

	// predefined strings for command help in telnet session
	commandHelp = "available commands: /help, /join [channel], /part, /ignore [user]"
	joinHelp    = "usage: /join [channel]"
	ignoreHelp  = "usage: /ignore [user]"
)

// translates plain text color to an escape sequence
func getTelnetColor(color string) string {
	if telnetColor, ok := TELNET_USERNAME_COLORS[color]; ok {
		return telnetColor
	} else {
		return DEFAULT_TELNET_USERNAME_COLOR
	}
}

type Telnet struct {
	// make name and channel json decodeable for other transports
	Name        string `json:"username"`
	Chan        string `json:"channel"`
	color       string
	telnetColor string
	conn        net.Conn
	ignoreList  map[string]bool

	// buffer is used for redrawing the terminal when new messages come
	// in or the window is resized
	buffer     [][]byte
	bufferSize int
	bufferMtx  sync.Mutex

	// stored width and height for redrawing terminal
	width  int
	height int

	//used to identify clients we can assert sizes for
	richClient bool
}

// helper method to create new telnet session
func NewTelnet(conn net.Conn, bufferSize int, usernameColor, channel string) *Telnet {
	return &Telnet{conn: conn, richClient: false, bufferSize: bufferSize,
		color: usernameColor, Chan: channel, ignoreList: make(map[string]bool)}
}

func (s *Telnet) Channel() string {
	return s.Chan
}

func (s *Telnet) IgnoreList() map[string]bool {
	return s.ignoreList
}

func (s *Telnet) Close() error {
	return s.conn.Close()
}

// helper method to add appropriate metadata to message from telnet session
func (s *Telnet) newMessage(bodyBytes []byte) Message {
	//filter out inappropriate bytes
	filteredBodyBytes := bodyBytes[:0]
	for _, b := range bodyBytes {
		octet := int(b)
		//other than CR and LF ensure chars are a restricted set of visual ascii
		if octet == 10 || octet == 13 || octet >= 32 && octet <= 126 {
			filteredBodyBytes = append(filteredBodyBytes, b)
		}
	}

	body := string(filteredBodyBytes)

	return NewMessage(string(body), s.Channel(), s)
}

func (s *Telnet) GetMessages() (msg, event chan Message, done chan error) {
	msg = make(chan Message)
	event = make(chan Message)
	done = make(chan error, 1)

	// we setup naws and username before giving the server a chance
	// to send us messages or receive them
	err := s.naws()
	if err != nil {
		// preload done so the server removes the session
		done <- err
		return msg, event, done
	}

	err = s.getUsername()
	if err != nil {
		// preload done so the server removes the session
		done <- err
		return msg, event, done
	}

	go func() {
		// do a fresh redraw on session setup
		err = s.redrawAll()
		if err != nil {
			done <- err
			return
		}

		b := make([]byte, EXPECTED_MSG_SIZE)
		for {
			n, err := s.conn.Read(b)
			// bail if we get an error when reading
			if err != nil {
				done <- err
				return
			}
			if n > 0 {

				// if we have a rich client look out for NAWS updates
				if s.richClient {
					nawsUpdate, err := s.handleNawsUpdates(b[:n])
					if err != nil {
						done <- err
						return
					}

					if nawsUpdate {
						continue
					}
				}

				// attend to any commands before creating a new message
				wasCommand, err := s.parseCommand(b[:n])
				if err != nil {
					done <- err
					return
				}

				if wasCommand {
					continue
				}

				m := s.newMessage(b[:n])
				msg <- m

				// redraw after a new message processed so the
				// user gets their compose window back
				err = s.redrawAll()
				if err != nil {
					done <- err
					return
				}
			}
		}
	}()

	return msg, event, done
}

// allows us to write raw bytes to the user
func (s *Telnet) raw(msg []byte) (err error) {
	_, err = s.conn.Write(msg)
	return err
}

// helper to add messages to buffer
func (s *Telnet) appendToBuffer(line string) {
	s.bufferMtx.Lock()
	defer s.bufferMtx.Unlock()

	// if existing buffer is smaller than bufferSize change end to avoid
	// nonexistant index accessing
	end := s.bufferSize
	if len(s.buffer) < end {
		end = len(s.buffer)
	}
	s.buffer = append([][]byte{[]byte(line)}, s.buffer[:end]...)
}

func (s *Telnet) SendMessage(msg Message) (err error) {
	body := msg.Body
	from := msg.From

	body = EVENT_COLOR + "[" + msg.T.Format("15:04:05") + "] " +
		getTelnetColor(from.UsernameColor()) + from.Username() + ": " +
		MESSAGE_COLOR + body

	s.appendToBuffer(body)
	return s.redrawChat()
}

func (s *Telnet) SendEvent(event Message) (err error) {
	msg := event.Body
	msg = EVENT_COLOR + msg + MESSAGE_COLOR
	s.appendToBuffer(msg)
	return s.redrawChat()
}

func (s *Telnet) Username() string {
	return s.Name
}

func (s *Telnet) UsernameColor() string {
	return s.color
}

func (s *Telnet) getUsername() (err error) {
	// clear screen for formatting
	err = s.clearScreen()
	if err != nil {
		return err
	}

	s.raw([]byte("username: "))
	b := make([]byte, EXPECTED_MSG_SIZE)

	// read bytes until we get an appropriate username ascii set
	for {
		n, err := s.conn.Read(b)
		if err != nil {
			return err
		}

		input := b[:n]

		if n > 0 && b[0] != IAC && len(strings.TrimSpace(string(input))) != 0 {
			username := strings.TrimSpace(string(input))
			s.Name = username
			err = s.clearScreen()
			return err
		}
	}
}

// Determine window size of session terminal
// [N]egotiate [A]bout [W]indow [S]ize
func (s *Telnet) naws() error {
	// inform client we want to do NAWS
	s.raw([]byte{IAC, DO, NAWS})
	b := make([]byte, EXPECTED_MSG_SIZE)
	for {
		n, err := s.conn.Read(b)
		if err != nil {
			return err
		}

		if n == 0 {
			continue
		}

		// not a command, continue until we get a response
		if b[0] != IAC {
			continue
		}

		// if we can do naws, set rich client status to true and
		// handle the naws update
		if b[1] == WILL && b[2] == NAWS {
			s.richClient = true
			if n > 3 {
				s.handleNawsUpdates(b[3:])
			}
			break
			// if client refused to do naws set rich client to false and break
		} else if b[1] == WONT && b[2] == NAWS {
			s.richClient = false
			break
		}
	}
	return nil
}

// sends clear screen escape sequence to terminal
func (s *Telnet) clearScreen() (err error) {
	_, err = s.conn.Write([]byte("\033[2J\033[0;0H"))
	return err
}

// resize virtual terminal info when we get naws info
func (s *Telnet) handleNawsUpdates(b []byte) (isNaws bool, err error) {
	if b[0] == IAC && b[1] == SB && b[2] == NAWS {
		width := 0
		if int(b[3]) == 1 {
			width += 256
		}
		width += int(b[4])

		height := 0
		if int(b[5]) == 1 {
			height += 256
		}
		height += int(b[6])

		s.width = width
		s.height = height

		// redraw based on new size info
		s.redrawChat()
		return true, err
	}
	return false, nil
}

// inform user to the status of their ignoring a certain user
func (s *Telnet) displayIgnoreStatus(user string) (err error) {
	if s.ignoreList[user] {
		return s.SendEvent(s.newMessage([]byte(user +
			" is now being ignored.")))
	} else {
		return s.SendEvent(s.newMessage([]byte(user +
			" is no longer being ignored.")))

	}
}

// helper for finding and parsing commands in input from users
func (s *Telnet) parseCommand(b []byte) (isCommand bool, err error) {
	// all commands begin with "/", bail otherwise
	if string(b[0]) != "/" {
		return false, nil
	}

	// get all arguments
	cmd := strings.Split(string(b), " ")

	switch strings.TrimSpace(cmd[0]) {
	case "/help":
		// show all commands when /help typed
		err = s.SendEvent(s.newMessage([]byte(commandHelp)))
		if err != nil {
			return true, err
		}
	case "/part":
		// close connection of /part received
		err = s.Close()
		return true, err
	case "/join":
		if len(cmd) < 2 || len(cmd[1]) == 0 {
			//bad usage of join, inform user of proper usage
			err = s.SendEvent(s.newMessage([]byte(joinHelp)))
			if err != nil {
				return true, err
			}
		} else {
			// update session's channel for messages
			s.Chan = string(strings.TrimSpace(cmd[1]))
			err = s.SendEvent(s.newMessage([]byte("now in channel #" +
				s.Channel())))
			if err != nil {
				return true, err
			}
		}
	case "/ignore":
		if len(cmd) < 2 || len(cmd[1]) == 0 {
			// if inappropriate args sent for ignore
			// inform user of proper usage
			err = s.SendEvent(s.newMessage([]byte(ignoreHelp)))
			if err != nil {
				return true, err
			}
		} else {
			user := string(strings.TrimSpace(cmd[1]))
			if _, ok := s.ignoreList[user]; ok {
				// allow user to stop ignoring a user by doing
				// /ignore [user] again
				s.ignoreList[user] = !s.ignoreList[user]
			} else {
				s.ignoreList[user] = true
			}

			err = s.displayIgnoreStatus(user)
			if err != nil {
				return true, err
			}
		}
	default:
		return false, nil
	}

	err = s.redrawAll()
	return true, err
}

// generate payload of bytes for redrawing chat window
func (s *Telnet) redrawChatBytes() []byte {
	// batch all writes into a single payload so we only write to
	// the client once

	// sequence to move cursor to top left of terminal
	payload := []byte("\033[0;0H")

	s.bufferMtx.Lock()
	defer s.bufferMtx.Unlock()

	// last line should stop before compose window
	lastLine := s.height - 1
	existingBufferLength := len(s.buffer)

	for i := 0; i <= lastLine; i++ {
		// for each line, we jump the cursor to that position
		// and clear the line
		payload = append(payload,
			[]byte("\033["+strconv.Itoa(i)+";0H\033[K")...)
		idx := lastLine - i

		// if we have a message for that line, add it to the buffer
		// otherwise it remains an empty line
		if idx < existingBufferLength {
			payload = append(payload, s.buffer[idx]...)
		}

	}
	return payload
}

func (s *Telnet) redrawChat() (err error) {
	// don't attempt to redraw chat if we don't know the size
	if !s.richClient {
		return nil
	}
	// save the cursor position (\033[s) (in the event a message was being composed)
	// redraw the chat portion
	// and then pop off the cursor (\033[u) position so the user has a seamless
	// composing experience while new messages pour in
	payload := append([]byte("\033[s"), append(s.redrawChatBytes(),
		[]byte("\033[u")...)...)
	return s.raw(payload)
}

// same as redrawChat except it redraws the compose window as well
func (s *Telnet) redrawAll() (err error) {
	if !s.richClient {
		return nil
	}
	payload := s.redrawChatBytes()
	payload = append(payload, []byte("\033["+strconv.Itoa(s.height)+";0H\033[K"+
		EVENT_COLOR+"[#"+s.Channel()+"] "+MESSAGE_COLOR)...)
	return s.raw(payload)
}
