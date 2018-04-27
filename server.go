package main

import (
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

type EVENT_TYPE int

const (
	USER_JOIN EVENT_TYPE = iota
	USER_LEAVE
	MSG_SENT
)

type Server struct {
	sessions                []*Session
	sessionBufferSize       int
	sessionLock             sync.Mutex
	chatlog, eventlog       *os.File
	chatlogMtx, eventlogMtx sync.Mutex
	usernameColors          []string
	colorMtx                sync.Mutex
	minimumMessageSize      int
}

func NewServer(chatlog, eventlog *os.File, sessionBufferSize int,
	usernameColors []string, minimumMessageSize int) *Server {
	return &Server{chatlog: chatlog, eventlog: eventlog,
		sessionBufferSize: sessionBufferSize, usernameColors: usernameColors,
		minimumMessageSize: minimumMessageSize}
}

func (s *Server) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		handleError(err)
		return err
	}

	log.Println("Listening on ", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			handleError(err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) logMessage(msg Message) (err error) {
	s.chatlogMtx.Lock()
	defer s.chatlogMtx.Unlock()
	_, err = s.chatlog.Write([]byte(strconv.FormatInt(msg.T.UnixNano(), 10) + ":" +
		msg.From.username + ":" + msg.Body))
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

func (s *Server) appendSession(sesh *Session) (sIdx int) {
	s.sessionLock.Lock()
	defer s.sessionLock.Unlock()
	s.sessions = append(s.sessions, sesh)
	return len(s.sessions) - 1
}

// Ensures connection is closed and then removed from list of sessions
func (s *Server) removeSession(sesh *Session) {
	s.sessionLock.Lock()
	//TODO: make dead session lookup more efficient
	for idx, ss := range s.sessions {
		if ss == sesh {
			s.sessions = append(s.sessions[:idx], s.sessions[idx+1:]...)
			break
		}
	}
	s.sessionLock.Unlock()

	s.event(NewMessage(sesh.username+" has left", nil))
}

func (s *Server) getUsernameColor() (color string) {
	s.colorMtx.Lock()
	defer s.colorMtx.Unlock()
	color = s.usernameColors[0]
	s.usernameColors = append(s.usernameColors[1:], color)
	return color
}

func (s *Server) handleConnection(conn net.Conn) {
	session := NewSession(conn, s.sessionBufferSize, s.getUsernameColor())

	s.appendSession(session)

	msgChan, eventChan, doneChan := session.GetMessages()
	var msg, event Message
	for {
		select {
		case msg = <-msgChan:
			s.broadcast(msg)
		case event = <-eventChan:
			s.event(event)
		case <-doneChan:
			s.removeSession(session)
			break
		}
	}
}

func (s *Server) broadcast(msg Message) {
	if len(strings.TrimSpace(msg.Body)) < s.minimumMessageSize {
		return
	}
	s.logMessage(msg)

	var failedSessions []*Session
	s.sessionLock.Lock()
	for _, sesh := range s.sessions {
		err := sesh.Send(msg)
		if err != nil {

		}
	}
	s.sessionLock.Unlock()

	for _, sesh := range failedSessions {
		s.removeSession(sesh)
	}
}

func (s *Server) event(event Message) {
	s.sessionLock.Lock()
	defer s.sessionLock.Unlock()
	for _, sesh := range s.sessions {
		sesh.SendEvent(event)
	}
}
