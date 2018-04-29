# Wally Chat
 
A simple chat server served over telnet

## How to run
Using flags:
`wally-chat -address=":9876" -chatlog_file="./chat.log" ...`

or with flagfile:
`wally-chat -flagfile=./wally-chat.flags`

where `wally-chat.flags` looks like this

```
address = 9876
chatlog_file = ./chat.log
sessionBufferSize = 20
minimumMessageLength = 1
defaultChannel = general
```

## My Approach
The server only works with raw TCP and assumes a telnet connection. I did not
get to implementing any http interfaces as I spent most of the time 
(possibly too much) on ux for a terminal session. Ansi escape sequences are
used heavily for drawing the chat interface including username colors, compose
window, window resizing, and alerts for new messages.

The server keeps an in memory list of sessions. If attempting to broadcast
to a session and the result is unsuccessful we just remove the session. The
server trusts the session metadata with regards to the channel it's in as well
as users it would like to ignore. The server logs each message including time,
body of message, username, as well as the channel it was sent in. It would be
easy enough to behave like a bouncer and update users to the last messages sent
in a channel when they joined but I ran out of time before I could implement
such logic.

The server's primary role is to accept new connections and distribute new
messages to all appropriate clients as they come in.

Telnet sessions do their best to smooth out the experience of joining and
sending/receiving messages. Great pains were taken to use pleasing formatting
while also avoiding allowing users to send malicious escape or control
sequences.

The Session interface allows new session types to be created as long as they
adhere to the protocol.

Sessions and Messages are json compatible for future http implementations.

## Commands
- /help (list commands)
- /join [channel] (join new channel)
- /ignore [user] (mute/unmute user)
- /part (disconnect)

## Limitations
- could use more tests
- Does not support HTTP REST endpoints
- does not support UTF{8,16} characters
- inefficient session removal (has to iterate through sessions until it finds the right one to remove)
- No existing tech to ensure horizontal scaling
- potential race condition when a message comes in *while* typing, could break visual continuation of composed message
- timestamps are only relative to server 
- can't see when a user switches room (only when they disconnect and connect)
- escape sequence colors may render poorly on unforseen terminal setups
- insufficient testing around terminals with _no_  NAWS capabilities (typically hardcoded ON with terminals)
- no cooldown for new messages, could potentially overwhelm server or other clients with a malicious client
- no duplicate username prevention
- no security (TELNETS or passwords)
- ignore only works on usernames, not ips

## 3rd Party Libs
- [spacemonkeygo/flagfile](https://github.com/spacemonkeygo/flagfile) (used for local file configuration loading)
