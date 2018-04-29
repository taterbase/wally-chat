package main

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/taterbase/wally-chat/session"
)

var (
	_ session.Session = (*mockSession)(nil)

	testChannel = "test"
)

type mockLogger struct {
	logs [][]byte
}

func (l *mockLogger) Write(p []byte) (n int, err error) {
	l.logs = append(l.logs, p)
	return len(p), nil
}

type mockSession struct {
	shouldFail bool
	username   string
	ignoreList map[string]bool
	messages   []session.Message
}

func (ms *mockSession) Channel() string {
	return testChannel
}

func (ms *mockSession) IgnoreList() map[string]bool {
	return ms.ignoreList
}

func (ms *mockSession) Username() string {
	return ms.username
}

func (ms *mockSession) UsernameColor() string {
	return "fuschia"
}

func (ms *mockSession) GetMessages(func(string) bool) (msg, event chan session.Message, done chan error) {
	msg = make(chan session.Message)
	event = make(chan session.Message)
	done = make(chan error)
	return msg, event, done
}

func (ms *mockSession) SendMessage(msg session.Message) error {
	if ms.shouldFail {
		return errors.New("I failed")
	}
	ms.messages = append(ms.messages, msg)
	return nil
}

func (ms *mockSession) SendEvent(session.Message) error {
	return nil
}

func (ms *mockSession) Close() error {
	return nil
}

func createMockSession(username string) *mockSession {
	if len(username) == 0 {
		username = "testuser"
	}

	return &mockSession{username: username, ignoreList: make(map[string]bool)}
}

func createMocks() (*mockLogger, *mockSession, *Server) {
	logger := &mockLogger{}
	sesh := createMockSession("testuser")
	s := NewServer(logger, 0, []string{}, 1, testChannel)
	return logger, sesh, s
}

func TestLogging(t *testing.T) {
	logger, sesh, s := createMocks()
	s.broadcast(session.NewMessage("test", testChannel, sesh), MESSAGE)
	if len(logger.logs) != 1 {
		t.Errorf("No logs written")
	}
}

func TestSessionAppending(t *testing.T) {
	_, sesh, s := createMocks()
	s.appendSession(sesh)
	if len(s.sessions) != 1 {
		t.Errorf("No sessions appended")
	}
}

func TestSessionDeparture(t *testing.T) {
	_, sesh, s := createMocks()
	s.appendSession(sesh)
	if len(s.sessions) != 1 {
		t.Errorf("No sessions appended")
	}

	sesh.shouldFail = true

	s.broadcast(session.NewMessage("test", testChannel, sesh), MESSAGE)
	if len(s.sessions) != 0 {
		t.Errorf("Sessions not removed even though it failed")
	}
}

func TestEventsDontShowUpInChatLog(t *testing.T) {
	logger, sesh, s := createMocks()
	s.broadcast(session.NewMessage("test", testChannel, sesh), EVENT)
	if len(logger.logs) != 0 {
		t.Errorf("event log written to chat log")
	}
}

func TestShouldRespectIgnoreList(t *testing.T) {
	_, _, s := createMocks()
	sesh1 := createMockSession("dan")
	sesh2 := createMockSession("jon")
	sesh1.ignoreList[sesh2.Username()] = true

	s.appendSession(sesh1)
	s.appendSession(sesh2)

	s.broadcast(session.NewMessage("test", testChannel, sesh2), MESSAGE)
	if len(sesh2.messages) == 0 {
		t.Errorf("message not broadcasted appropriately")
	}
	if len(sesh1.messages) != 0 {
		t.Errorf("message not ignored appropriately")
	}
}

func TestShouldSeparateRecordsAppropriately(t *testing.T) {
	logger, sesh, s := createMocks()
	s.appendSession(sesh)
	msg := session.NewMessage("test", testChannel, sesh)
	s.broadcast(msg, MESSAGE)

	row := string(logger.logs[0])
	pieces := strings.Split(row, RECORD_SEPARATOR)
	if len(pieces) != 4 {
		t.Errorf("incorrect number of records in chat row %d", len(pieces))
	}

	if pieces[0] != strconv.FormatInt(msg.T.UnixNano(), 10) {
		t.Errorf("first record is not timestamp %s", pieces[0])
	}

	if pieces[1] != msg.Channel {
		t.Errorf("second record is not channel %s", pieces[1])
	}

	if pieces[2] != msg.From.Username() {
		t.Errorf("third record is not username %s", pieces[2])
	}

	if pieces[3] != msg.Body {
		t.Errorf("fourth record is not body %s", pieces[3])
	}
}
