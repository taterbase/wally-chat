package main

import (
	"flag"
	"os"

	"github.com/spacemonkeygo/flagfile"
)

var (
	address     = flag.String("address", ":9876", "address for chat server to listen in on")
	chatlogFile = flag.String("chatlog_file", "./chat.log",
		"the file to log all messages to (created if does not already exist")
	eventlogFile = flag.String("eventlog_file", "./event.log",
		"the file to log all events to (created if does not already exist")
	sessionBufferSize = flag.Int("session_buffer_size", 20,
		"Limit of messages held in memory buffer for session")
	minimumMessageLength = flag.Int("minimum_message_length", 1,
		"The minimum characters required for a message")

	USERNAME_COLORS = []string{
		"\033[0;34m", "\033[0;35m",
		"\033[0;33m", "\033[1;34m", "\033[1;32m", "\033[1;36m", "\033[1;31m",
		"\033[1;35m", "\033[1;33m"}
)

//TODO(george): Do something cleaner dude
func handleError(err error) {
	panic(err)
}

func main() {
	flagfile.Load()

	chatLog, err := os.OpenFile(*chatlogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644)
	if err != nil {
		handleError(err)
	}

	eventLog, err := os.OpenFile(*eventlogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644)
	if err != nil {
		handleError(err)
	}

	server := NewServer(chatLog, eventLog, *sessionBufferSize, USERNAME_COLORS,
		*minimumMessageLength)
	server.Listen(*address)
}
