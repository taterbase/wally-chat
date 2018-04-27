package main

import (
	"fmt"
	"math/rand"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
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
	endsWithReturn  = regexp.MustCompile("(\r\n|\r|\n)")
	USERNAME_COLORS = []string{
		"\033[0;36m", "\033[0;34m", "\033[0;35m",
		"\033[0;33m", "\033[1;34m", "\033[1;32m", "\033[1;36m", "\033[1;31m",
		"\033[1;35m", "\033[1;33m"}
	MESSAGE_COLOR = "\033[1;37m"
	EVENT_COLOR   = "\033[1;30m"
)

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

func NewSession(conn net.Conn, bufferSize int) *Session {
	return &Session{conn: conn, richClient: false, bufferSize: bufferSize}
}

func (s *Session) Close() error {
	return s.conn.Close()
}

func (s *Session) GetMessages() (msg, event chan string, done chan error) {
	msg = make(chan string)
	event = make(chan string)
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

		event <- s.username + " has joined"

		b := make([]byte, EXPECTED_MSG_SIZE)
		for {
			n, err := s.conn.Read(b)
			if err != nil {
				done <- err
				return
			}
			if n > 0 {
				if s.richClient {
					fmt.Println("neato")
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
				msg <- string(b[:n])
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

func (s *Session) Send(msg string, from *Session) (err error) {
	if !endsWithReturn.MatchString(msg) {
		msg = msg + "\r\n"
	}

	if from != nil {
		msg = from.color + from.username + ": " + MESSAGE_COLOR + msg
	}

	s.appendToBuffer(msg)
	return s.redrawChat()
}

func (s *Session) SendEvent(event string) (err error) {
	if !endsWithReturn.MatchString(event) {
		event = event + "\r\n"
	}

	event = EVENT_COLOR + event + MESSAGE_COLOR
	s.appendToBuffer(event)
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
			s.color = USERNAME_COLORS[rand.Int()%len(USERNAME_COLORS)]
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
			fmt.Println("aww hell naws")
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
	fmt.Println("cool", int(b[1]))
	if b[0] == IAC && b[1] == SB && b[2] == NAWS {
		fmt.Println("Even cooler")
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
		fmt.Println(width, height)
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
