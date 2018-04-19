package main

import (
	"flag"

	"github.com/spacemonkeygo/flagfile"
)

var (
	address = flag.String("address", ":9876", "address for chat server to listen in on")
)

//TODO(george): Do something cleaner dude
func handleError(err error) {
	panic(err)
}

func main() {
	flagfile.Load()

	server := NewServer()
	server.Listen(*address)
}
