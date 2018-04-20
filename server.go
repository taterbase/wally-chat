package main

import (
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
)

type EVENT_TYPE int

const (
	USER_JOIN EVENT_TYPE = iota
	USER_LEAVE
	MSG_SENT
)

var (
	endsWithReturn  = regexp.MustCompile("(\r\n|\r|\n)")
	USERNAME_COLORS = []string{
		"\033[0;36m", "\033[0;34m", "\033[0;35m",
		"\033[0;33m", "\033[1;34m", "\033[1;32m", "\033[1;36m", "\033[1;31m",
		"\033[1;35m", "\033[1;33m"}
	MESSAGE_COLOR      = "\033[1;37m"
	INTRODUCTION_COLOR = "\033[1;30m"
)

type Session struct {
	username string
	color    string
	conn     net.Conn
}

func NewSession(conn net.Conn) *Session {
	return &Session{conn: conn}
}

func (s *Session) updateUsername(username string) error {
	username = strings.TrimSpace(username)
	log.Println("New username", username)
	s.username = username
	s.color = USERNAME_COLORS[rand.Int()%len(USERNAME_COLORS)]
	return nil
}

func (s *Session) send(msg string) (err error) {
	//TODO(george): need to detect if return character is already there
	//if not add one
	_, err = s.conn.Write([]byte(msg))
	log.Println("BROADCASTING", msg)
	return err
}

func (s *Session) clearScreen() (err error) {
	_, err = s.conn.Write([]byte("\033[2J\033[1;1H"))
	return err
}

type Server struct {
	sessions          []*Session
	sessionLock       sync.Mutex
	chatlog, eventlog *os.File
}

func NewServer(chatlog, eventlog *os.File) *Server {
	return &Server{chatlog: chatlog, eventlog: eventlog}
}

func (s *Server) Listen(addr string) error {
	ln, err := net.Listen("tcp", *address)
	if err != nil {
		handleError(err)
		return err
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			handleError(err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) logMessage(msg string) (err error) {
	_, err = s.chatlog.Write([]byte(msg))
	return err
}

func (s *Server) logEvent(event EVENT_TYPE) (err error) {
	switch event {
	case USER_JOIN:
		_, err = s.eventlog.Write([]byte("user:join"))
	case USER_LEAVE:
		_, err = s.eventlog.Write([]byte("user:leave"))
	case MSG_SENT:
		_, err = s.eventlog.Write([]byte("msg:sent"))
		//TODO(george): add default
	}
	return err
}

func (s *Server) raw(msg string, from *Session) (err error) {
	for _, sesh := range s.sessions {
		if sesh != from {
			err = sesh.send(msg)
		}
	}
	return err
}

func (s *Server) broadcast(msg string, from *Session) (err error) {
	if !endsWithReturn.MatchString(msg) {
		msg = msg + "\n"
	}

	if from != nil {
		msg = from.color + from.username + ": " + MESSAGE_COLOR + msg
	}

	err = s.raw(msg, from)
	if err != nil {
		s.logMessage(msg)
		s.logEvent(MSG_SENT)
	}
	return err
}

func (s *Server) appendSession(sesh *Session) error {
	s.sessionLock.Lock()
	defer s.sessionLock.Unlock()
	s.sessions = append(s.sessions, sesh)
	return nil
}

func (s *Server) introduce(sesh *Session) (err error) {
	s.raw(INTRODUCTION_COLOR, nil)
	s.raw("! "+sesh.username+" has joined\n", nil)
	s.raw(MESSAGE_COLOR, nil)
	s.logEvent(USER_JOIN)
	return err
}

func (s *Server) handleConnection(conn net.Conn) {
	log.Println("New connection")

	session := NewSession(conn)

	session.clearScreen()
	conn.Write([]byte("Welcome to wally chat. Please enter a username: "))

	b := make([]byte, 128)
	for {
		n, err := conn.Read(b)
		if err != nil {
			if err == io.EOF {
				log.Println("Connection closed by client")
				return
			}

			handleError(err)
			continue
		}

		if n > 0 {
			msg := string(b[:n])
			if session.username == "" {
				err = session.updateUsername(msg)
				if err != nil {
					handleError(err)
					//TODO(george): need to inform user error happened
					conn.Close()
					break
				}
				session.clearScreen()
				s.appendSession(session)
				s.introduce(session)
				continue
			}
			log.Println("some dater")
			s.broadcast(msg, session)
		}
	}
}
