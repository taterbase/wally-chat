package main

import (
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/taterbase/wally-chat/session"
)

type BROADCAST_TYPE int

const (
	MESSAGE BROADCAST_TYPE = iota
	EVENT

	RECORD_SEPARATOR = "\036"
)

type Server struct {
	defaultChannel          string
	sessions                []session.Session
	sessionBufferSize       int
	sessionLock             sync.Mutex
	chatlog, eventlog       *os.File
	chatlogMtx, eventlogMtx sync.Mutex
	usernameColors          []string
	colorMtx                sync.Mutex
	minimumMessageSize      int
}

func NewServer(chatlog, eventlog *os.File, sessionBufferSize int,
	usernameColors []string, minimumMessageSize int, defaultChannel string) *Server {
	return &Server{chatlog: chatlog, eventlog: eventlog,
		sessionBufferSize: sessionBufferSize, usernameColors: usernameColors,
		minimumMessageSize: minimumMessageSize, defaultChannel: defaultChannel}
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

func (s *Server) logMessage(msg session.Message) (err error) {
	s.chatlogMtx.Lock()
	defer s.chatlogMtx.Unlock()
	_, err = s.chatlog.Write([]byte(strconv.FormatInt(msg.T.UnixNano(), 10) +
		RECORD_SEPARATOR + msg.Channel + RECORD_SEPARATOR +
		msg.From.Username() + RECORD_SEPARATOR + msg.Body))
	return err
}

func (s *Server) appendSession(sesh session.Session) {
	s.sessionLock.Lock()
	s.sessions = append(s.sessions, sesh)
	s.sessionLock.Unlock()
	s.broadcast(session.NewMessage(sesh.Username()+" is now online",
		sesh.Channel(), sesh), EVENT)
}

// Ensures connection is closed and then removed from list of sessions
func (s *Server) removeSession(sesh session.Session) {
	s.sessionLock.Lock()
	//TODO: make dead session lookup more efficient
	for idx, ss := range s.sessions {
		if ss == sesh {
			s.sessions = append(s.sessions[:idx], s.sessions[idx+1:]...)
			break
		}
	}
	s.sessionLock.Unlock()

	s.broadcast(session.NewMessage(sesh.Username()+" has disconnected",
		sesh.Channel(), sesh), EVENT)
}

func (s *Server) getUsernameColor() (color string) {
	s.colorMtx.Lock()
	defer s.colorMtx.Unlock()
	color = s.usernameColors[0]
	s.usernameColors = append(s.usernameColors[1:], color)
	return color
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	sesh := session.NewTelnet(conn, s.sessionBufferSize, s.getUsernameColor(),
		s.defaultChannel)

	msgChan, eventChan, doneChan := sesh.GetMessages()
	s.appendSession(sesh)
	var msg, event session.Message
	for {
		select {
		case msg = <-msgChan:
			s.broadcast(msg, MESSAGE)
		case event = <-eventChan:
			s.broadcast(event, EVENT)
		case <-doneChan:
			s.removeSession(sesh)
			break
		}
	}
}

func (s *Server) broadcast(msg session.Message, bt BROADCAST_TYPE) {
	if len(strings.TrimSpace(msg.Body)) < s.minimumMessageSize {
		return
	}

	if bt == MESSAGE {
		s.logMessage(msg)
	}

	var failedSessions []session.Session
	var err error
	s.sessionLock.Lock()
	for _, sesh := range s.sessions {
		if sesh.Channel() != msg.Channel {
			continue
		}

		if isIgnored, ok := sesh.IgnoreList()[msg.From.Username()]; ok {
			if isIgnored {
				continue
			}
		}

		switch bt {
		case MESSAGE:
			err = sesh.SendMessage(msg)
		case EVENT:
			err = sesh.SendEvent(msg)
		}
		if err != nil {
			failedSessions = append(failedSessions, sesh)
		}
	}
	s.sessionLock.Unlock()

	for _, sesh := range failedSessions {
		s.removeSession(sesh)
	}
}
