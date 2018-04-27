package session

import (
	"math/rand"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	IAC  = byte(255)
	DONT = byte(254)
	DO   = byte(253)
	WONT = byte(252)
	WILL = byte(251)
	SB   = byte(250)
	SE   = byte(240)
	NAWS = byte(31)

	EXPECTED_MSG_SIZE = 128
)

var (
	endsWithReturn          = regexp.MustCompile("(\r\n|\r|\n)")
	MESSAGE_COLOR           = "\033[1;37m"
	EVENT_COLOR             = "\033[1;30m"
	usernameColorRandomizer = rand.New(rand.NewSource(time.Now().UnixNano()))

	_ Session = (*Telnet)(nil)

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

	DEFAULT_TELNET_USERNAME_COLOR = TELNET_USERNAME_COLORS["fuschia"]

	commandHelp = "available commands: /help, /join [channel], /part, /ignore [user]"
	joinHelp    = "usage: /join [channel]"
	ignoreHelp  = "usage: /ignore [user]"
)

func getTelnetColor(color string) string {
	if telnetColor, ok := TELNET_USERNAME_COLORS[color]; ok {
		return telnetColor
	} else {
		return DEFAULT_TELNET_USERNAME_COLOR
	}
}

type Telnet struct {
	username    string
	channel     string
	color       string
	telnetColor string
	conn        net.Conn
	richClient  bool
	buffer      [][]byte
	bufferSize  int
	bufferMtx   sync.Mutex
	width       int
	height      int
	ignoreList  map[string]bool
}

func NewTelnet(conn net.Conn, bufferSize int, usernameColor, channel string) *Telnet {
	return &Telnet{conn: conn, richClient: false, bufferSize: bufferSize,
		color: usernameColor, channel: channel, ignoreList: make(map[string]bool)}
}

func (s *Telnet) Channel() string {
	return s.channel
}

func (s *Telnet) IgnoreList() map[string]bool {
	return s.ignoreList
}

func (s *Telnet) Close() error {
	return s.conn.Close()
}

func (s *Telnet) newMessage(bodyBytes []byte) Message {
	filteredBodyBytes := bodyBytes[:0]
	for _, b := range bodyBytes {
		octet := int(b)
		if octet >= 32 && octet <= 126 {
			filteredBodyBytes = append(filteredBodyBytes, b)
		}
	}
	body := string(filteredBodyBytes)
	if !endsWithReturn.MatchString(body) {
		body = body + "\r\n"
	}

	return NewMessage(string(body), s.Channel(), s)
}

func (s *Telnet) GetMessages() (msg, event chan Message, done chan error) {
	msg = make(chan Message)
	event = make(chan Message)
	done = make(chan error, 1)

	err := s.naws()
	if err != nil {
		done <- err
		return msg, event, done
	}

	err = s.getUsername()
	if err != nil {
		done <- err
		return msg, event, done
	}

	go func() {
		err = s.redrawAll()
		if err != nil {
			done <- err
			return
		}

		b := make([]byte, EXPECTED_MSG_SIZE)
		for {
			n, err := s.conn.Read(b)
			if err != nil {
				done <- err
				return
			}
			if n > 0 {
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

func (s *Telnet) raw(msg []byte) (err error) {
	_, err = s.conn.Write(msg)
	return err
}

func (s *Telnet) appendToBuffer(line string) {
	s.bufferMtx.Lock()
	defer s.bufferMtx.Unlock()

	end := s.bufferSize
	if len(s.buffer) < end {
		end = len(s.buffer)
	}
	s.buffer = append([][]byte{[]byte(line)}, s.buffer[:end]...)
}

func (s *Telnet) SendMessage(msg Message) (err error) {
	body := msg.Body
	from := msg.From

	if from != nil {
		body = EVENT_COLOR + "[" + msg.T.Format("15:04:05") + "] " +
			getTelnetColor(from.UsernameColor()) + from.Username() + ": " +
			MESSAGE_COLOR + body
	}

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
	return s.username
}

func (s *Telnet) UsernameColor() string {
	return s.color
}

func (s *Telnet) getUsername() (err error) {
	err = s.clearScreen()
	if err != nil {
		return err
	}

	s.raw([]byte("username: "))
	b := make([]byte, EXPECTED_MSG_SIZE)
	for {
		n, err := s.conn.Read(b)
		if err != nil {
			return err
		}

		if n > 0 && b[0] != IAC {
			username := strings.TrimSpace(string(b[:n]))
			s.username = username
			err = s.clearScreen()
			return err
		}
	}
}

// Determine window size of session terminal
// [N]egotiate [A]bout [W]indow [S]ize
func (s *Telnet) naws() error {
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

		if b[0] != IAC {
			continue
		}

		if b[1] == WILL && b[2] == NAWS {
			s.richClient = true
			if n > 3 {
				s.handleNawsUpdates(b[3:])
			}
			break
		} else if b[1] == WONT && b[2] == NAWS {
			s.richClient = false
			break
		}
	}
	return nil
}

// sends clear screen escape sequence to terminal
func (s *Telnet) clearChat() (err error) {
	_, err = s.conn.Write([]byte("\033[2J"))
	return err
}

// sends clear screen escape sequence to terminal
func (s *Telnet) clearScreen() (err error) {
	_, err = s.conn.Write([]byte("\033[2J\033[0;0H"))
	return err
}

func (s *Telnet) handleNawsUpdates(b []byte) (isNaws bool, err error) {
	if b[0] == IAC && b[1] == SB && b[2] == NAWS {
		//update terminal size
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
		s.redrawChat()
		return true, err
	}
	return false, nil
}

func (s *Telnet) displayIgnoreStatus(user string) (err error) {
	if s.ignoreList[user] {
		return s.SendEvent(s.newMessage([]byte(user +
			" is now being ignored.")))
	} else {
		return s.SendEvent(s.newMessage([]byte(user +
			" is no longer being ignored.")))

	}
}

func (s *Telnet) parseCommand(b []byte) (isCommand bool, err error) {
	if string(b[0]) != "/" {
		return false, nil
	}

	cmd := strings.Split(string(b), " ")

	switch strings.TrimSpace(cmd[0]) {
	case "/help":
		err = s.SendEvent(s.newMessage([]byte(commandHelp)))
		if err != nil {
			return true, err
		}
	case "/part":
		err = s.Close()
		return true, err
	case "/join":
		channel := string(strings.TrimSpace(cmd[1]))
		if len(channel) == 0 {
			if err != nil {
				return true, err
			}
		} else {
			s.channel = channel
			err = s.SendEvent(s.newMessage([]byte("now in channel #" +
				channel)))
			if err != nil {
				return true, err
			}
		}
	case "/ignore":
		user := string(strings.TrimSpace(cmd[1]))
		if len(user) == 0 {
			err = s.SendEvent(s.newMessage([]byte(ignoreHelp)))
			if err != nil {
				return true, err
			}
		} else {
			if _, ok := s.ignoreList[user]; ok {
				s.ignoreList[user] = !s.ignoreList[user]
			} else {
				s.ignoreList[user] = true
			}

			if s.ignoreList[user] {
				err = s.SendEvent(s.newMessage([]byte(user +
					" is now being ignored.")))
			} else {
				err = s.SendEvent(s.newMessage([]byte(user +
					" is no longer being ignored.")))
			}

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

func (s *Telnet) redrawChatBytes() []byte {
	payload := []byte("\033[0;0H")

	s.bufferMtx.Lock()
	defer s.bufferMtx.Unlock()

	lastLine := s.height - 1
	existingBufferLength := len(s.buffer)

	for i := 0; i <= lastLine; i++ {
		payload = append(payload,
			[]byte("\033["+strconv.Itoa(i)+";0H\033[K")...)
		idx := lastLine - i
		if idx < existingBufferLength {
			payload = append(payload, s.buffer[idx]...)
		}
	}
	return payload
}

func (s *Telnet) redrawChat() (err error) {
	payload := append([]byte("\033[s"), append(s.redrawChatBytes(),
		[]byte("\033[u")...)...)
	return s.raw(payload)
}

func (s *Telnet) redrawAll() (err error) {
	payload := s.redrawChatBytes()
	payload = append(payload, []byte("\033["+strconv.Itoa(s.height)+";0H\033[K"+
		EVENT_COLOR+"[#"+s.Channel()+"] "+MESSAGE_COLOR)...)
	return s.raw(payload)
}
