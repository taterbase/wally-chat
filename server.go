package main

import (
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/taterbase/wally-chat/session"
)

type BROADCAST_TYPE int

const (
	MESSAGE BROADCAST_TYPE = iota
	EVENT

	// special ascii character specifically for separating records
	RECORD_SEPARATOR = "\036"
)

type Server struct {
	defaultChannel     string
	sessions           map[string]session.Session
	sessionBufferSize  int
	sessionLock        sync.Mutex
	chatlog            io.Writer
	chatlogMtx         sync.Mutex
	usernameColors     []string
	colorMtx           sync.Mutex
	minimumMessageSize int
}

// server creation helper method
func NewServer(chatlog io.Writer, sessionBufferSize int,
	usernameColors []string, minimumMessageSize int, defaultChannel string) *Server {
	return &Server{chatlog: chatlog, sessionBufferSize: sessionBufferSize,
		usernameColors: usernameColors, minimumMessageSize: minimumMessageSize,
		defaultChannel: defaultChannel,
		sessions:       make(map[string]session.Session)}
}

// kicks of server with appropriate address
func (s *Server) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	log.Println("Listening on ", addr)

	// basic loop for accepting new connections
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) UsernameAvailable(username string) bool {
	s.sessionLock.Lock()
	defer s.sessionLock.Unlock()
	if _, ok := s.sessions[username]; ok {
		return false
	}
	return true
}

// function responsible for logging all messages
func (s *Server) logMessage(msg session.Message) (err error) {
	// avoid chat log writing races
	s.chatlogMtx.Lock()
	defer s.chatlogMtx.Unlock()

	_, err = s.chatlog.Write([]byte(strconv.FormatInt(msg.T.UnixNano(), 10) +
		RECORD_SEPARATOR + msg.Channel + RECORD_SEPARATOR +
		msg.From.Username() + RECORD_SEPARATOR + msg.Body))
	return err
}

// function responsible for adding new sessions to the server
func (s *Server) appendSession(sesh session.Session) {
	s.sessionLock.Lock()
	s.sessions[sesh.Username()] = sesh
	s.sessionLock.Unlock()
	s.broadcast(session.NewMessage(sesh.Username()+" is now online",
		sesh.Channel(), sesh), EVENT)
}

// Ensures connection is closed and then removed from list of sessions
func (s *Server) removeSession(sesh session.Session) {
	s.sessionLock.Lock()
	sesh.Close()
	delete(s.sessions, sesh.Username())
	s.sessionLock.Unlock()

	s.broadcast(session.NewMessage(sesh.Username()+" has disconnected",
		sesh.Channel(), sesh), EVENT)
}

// assigns mostly unique (rotating set) color to session
func (s *Server) getUsernameColor() (color string) {
	// to help ensure uniqueness of username colors we lock access while
	// distributing new colors, and then shift the array for the next
	// session
	s.colorMtx.Lock()
	defer s.colorMtx.Unlock()
	color = s.usernameColors[0]
	s.usernameColors = append(s.usernameColors[1:], color)
	return color
}

// handles the logic of an open connection
// meant to be spun out in a goroutine
func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	sesh := session.NewTelnet(conn, s.sessionBufferSize, s.getUsernameColor(),
		s.defaultChannel)

	msgChan, eventChan, doneChan := sesh.GetMessages(s.UsernameAvailable)
	s.appendSession(sesh)
	var msg, event session.Message
	for {
		select {
		case msg = <-msgChan:
			// new message from session
			s.broadcast(msg, MESSAGE)
		case event = <-eventChan:
			// new event from session
			s.broadcast(event, EVENT)
		case <-doneChan:
			// session has told us it's done, remove it
			s.removeSession(sesh)
			break
		}
	}
}

// handles logic for sending messages to appropriate sessions
func (s *Server) broadcast(msg session.Message, bt BROADCAST_TYPE) {
	if len(strings.TrimSpace(msg.Body)) < s.minimumMessageSize {
		return
	}

	// if it's a message log it, otherwise don't record
	if bt == MESSAGE {
		s.logMessage(msg)
	}

	// we batch failed sessions for removal later. sessions are locked
	// during broadcast so we can't remove until the lock is released
	var failedSessions []session.Session
	var err error

	s.sessionLock.Lock()
	for _, sesh := range s.sessions {
		// if a message's channel is different from a session's don't show it
		if sesh.Channel() != msg.Channel {
			continue
		}

		// respect ignore list and don't broadcast from ignored sessions
		// TODO: simply username based, potentially include ip at later date
		if isIgnored, ok := sesh.IgnoreList()[msg.From.Username()]; ok {
			if isIgnored {
				continue
			}
		}

		// broadcast message based on type appropriately so sessions
		// can display with correct formatting to its user
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

	// now that the session lock has been released remove bad sessions
	for _, sesh := range failedSessions {
		s.removeSession(sesh)
	}
}
