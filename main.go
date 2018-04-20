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

	server := NewServer(chatLog, eventLog)
	server.Listen(*address)
}
