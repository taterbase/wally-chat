package main

import (
	"flag"
	"net"
	"os"
	"strconv"
	"sync"
)

type EVENT_TYPE int

const (
	USER_JOIN EVENT_TYPE = iota
	USER_LEAVE
	MSG_SENT
)

var (
	sessionBufferSize = flag.Int("session_buffer_size", 20,
		"Limit of messages held in memory buffer for session")
)

type Server struct {
	sessions                []*Session
	sessionLock             sync.Mutex
	chatlog, eventlog       *os.File
	chatlogMtx, eventlogMtx sync.Mutex
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
func (s *Server) removeSession(sIdx int) {
	s.sessionLock.Lock()
	defer s.sessionLock.Unlock()

	session := s.sessions[sIdx]
	session.Close()

	s.sessions = append(s.sessions[:sIdx], s.sessions[sIdx+1:]...)
}

func (s *Server) handleConnection(conn net.Conn) {
	session := NewSession(conn, *sessionBufferSize)

	sIdx := s.appendSession(session)
	defer s.removeSession(sIdx)

	msgChan, eventChan, doneChan := session.GetMessages()
	var msg, event Message
	for {
		select {
		case msg = <-msgChan:
			s.broadcast(msg)
		case event = <-eventChan:
			s.event(event)
		case <-doneChan:
			break
		}
	}
}

func (s *Server) broadcast(msg Message) {
	s.logMessage(msg)
	for _, sesh := range s.sessions {
		err := sesh.Send(msg)
		if err != nil {
			panic("TODO")
		}
	}
}

func (s *Server) event(event Message) {
	for _, sesh := range s.sessions {
		sesh.SendEvent(event)
	}
}
