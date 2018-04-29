package session

// Session interface allows us to add other types later (like http)
type Session interface {
	Channel() string
	IgnoreList() map[string]bool
	Username() string
	UsernameColor() string
	GetMessages(usernameAvilable func(string) bool) (msg, event chan Message,
		done chan error)
	SendMessage(Message) error
	SendEvent(Message) error
	Close() error
}
