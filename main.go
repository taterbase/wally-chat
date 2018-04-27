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
	defaultChannel = flag.String("default_channel", "general",
		"the first channel a user enters when they join")

	USERNAME_COLORS = []string{
		"red",
		"green",
		"brown",
		"blue",
		"purple",
		"cyan",
		"orange",
		"lime",
		"yellow",
		"indigo",
		"fuschia",
		"aqua",
	}
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
		*minimumMessageLength, *defaultChannel)
	server.Listen(*address)
}
