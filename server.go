package main

import (
	"io"
	"log"
	"net"
	"regexp"
	"strings"
	"sync"
)

var (
	endsWithReturn = regexp.MustCompile("(\r\n|\r|\n)")
)

type Session struct {
	username string
	conn     net.Conn
}

func NewSession(conn net.Conn) *Session {
	return &Session{conn: conn}
}

func (s *Session) updateUsername(username string) error {
	username = strings.TrimSpace(username)
	log.Println("New username", username)
	s.username = username
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
	sessions    []*Session
	sessionLock sync.Mutex
}

func NewServer() *Server {
	return &Server{}
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
		msg = from.username + ": " + msg
	}

	return s.raw(msg, from)
}

func (s *Server) appendSession(sesh *Session) error {
	s.sessionLock.Lock()
	defer s.sessionLock.Unlock()
	s.sessions = append(s.sessions, sesh)
	return nil
}

func (s *Server) introduce(sesh *Session) (err error) {
	s.raw("\033[1;30m", nil)
	s.broadcast("! "+sesh.username+" has joined", nil)
	err = s.raw("\033[1;37m", nil)
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
