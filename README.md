# Wally Chat
 
A simple chat server served over telnet

## My Approach
I was really excited for this project as I've always really enjoyed creating chat servers for hobby projects.
Initially I set out to create a simple telnet chat server that would simply

## Limitations
- Does not support HTTP REST endpoints
- does not support UTF{8,16} characters
- inefficient session removal (has to iterate through sessions until it finds the right one to remove)
- No existing tech to ensure horizontal scaling
- potential race condition when a message comes in *while* typing, could break visual continuation of composed message
- timestamps are only relative to server 
- can't see when a user switches room (only when they disconnect and connect)
- escape sequence colors may render poorly on unforseen terminal setups
- insufficient testing around terminals with _no_  NAWS capabilities (typically hardcoded ON with terminals)

## 3rd Party Libs
- [spacemonkeygo/flagfile](https://github.com/spacemonkeygo/flagfile) (used for local file configuration loading)
