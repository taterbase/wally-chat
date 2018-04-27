package main

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
)

type Message struct {
	T    time.Time
	From *Session
	Body string
}

func NewMessage(body string, from *Session) Message {
	return Message{
		Body: body,
		From: from,
		T:    time.Now(),
	}
}

type Session struct {
	username   string
	color      string
	conn       net.Conn
	richClient bool
	buffer     [][]byte
	bufferSize int
	bufferMtx  sync.Mutex
	width      int
	height     int
	sizeMtx    sync.Mutex
}

func NewSession(conn net.Conn, bufferSize int, usernameColor string) *Session {
	return &Session{conn: conn, richClient: false, bufferSize: bufferSize,
		color: usernameColor}
}

func (s *Session) Close() error {
	return s.conn.Close()
}

func (s *Session) newMessage(bodyBytes []byte) Message {
	body := string(bodyBytes)
	if !endsWithReturn.MatchString(body) {
		body = body + "\r\n"
	}

	return NewMessage(string(body), s)
}

func (s *Session) GetMessages() (msg, event chan Message, done chan error) {
	msg = make(chan Message)
	event = make(chan Message)
	done = make(chan error, 1)

	//TODO: pull out into named func
	go func() {
		err := s.naws()
		if err != nil {
			done <- err
			return
		}

		err = s.getUsername()
		if err != nil {
			done <- err
			return
		}

		event <- s.newMessage([]byte(s.username + " has joined"))
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
						s.redrawChat()
						continue
					}
				}
				//TODO: clean up inputs
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

func (s *Session) raw(msg []byte) (err error) {
	_, err = s.conn.Write(msg)
	return err
}

func (s *Session) appendToBuffer(line string) {
	s.bufferMtx.Lock()
	defer s.bufferMtx.Unlock()

	end := s.bufferSize
	if len(s.buffer) < end {
		end = len(s.buffer)
	}
	s.buffer = append([][]byte{[]byte(line)}, s.buffer[:end]...)
}

func (s *Session) Send(msg Message) (err error) {
	body := msg.Body
	from := msg.From

	if from != nil {
		body = EVENT_COLOR + "[" + msg.T.Format("15:04:05") + "] " +
			from.color + from.username + ": " + MESSAGE_COLOR + body
	}

	s.appendToBuffer(body)
	return s.redrawChat()
}

func (s *Session) SendEvent(event Message) (err error) {
	msg := event.Body
	msg = EVENT_COLOR + msg + MESSAGE_COLOR
	s.appendToBuffer(msg)
	return s.redrawChat()
}

func (s *Session) getUsername() (err error) {
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
func (s *Session) naws() error {
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
func (s *Session) clearChat() (err error) {
	_, err = s.conn.Write([]byte("\033[2J"))
	return err
}

// sends clear screen escape sequence to terminal
func (s *Session) clearScreen() (err error) {
	_, err = s.conn.Write([]byte("\033[2J\033[0;0H"))
	return err
}

func (s *Session) handleNawsUpdates(b []byte) (isNaws bool, err error) {
	if b[0] == IAC && b[1] == SB && b[2] == NAWS {
		s.sizeMtx.Lock()
		defer s.sizeMtx.Unlock()
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
		return true, err
	}
	return false, nil
}

func (s *Session) redrawChatBytes() []byte {
	payload := []byte("\033[0;0H")

	s.bufferMtx.Lock()
	defer s.bufferMtx.Unlock()

	s.sizeMtx.Lock()
	defer s.sizeMtx.Unlock()

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

func (s *Session) redrawChat() (err error) {
	payload := append([]byte("\033[s"), append(s.redrawChatBytes(),
		[]byte("\033[u")...)...)
	return s.raw(payload)
}

func (s *Session) redrawAll() (err error) {
	payload := s.redrawChatBytes()
	payload = append(payload, []byte("\033["+strconv.Itoa(s.height)+";0H\033[K"+
		EVENT_COLOR+"[#general] "+MESSAGE_COLOR)...)
	return s.raw(payload)
}
