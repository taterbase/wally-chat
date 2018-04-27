package session

type Session interface {
	Channel() string
	IgnoreList() map[string]bool
	Username() string
	UsernameColor() string
	GetMessages() (msg, event chan Message, done chan error)
	SendMessage(Message) error
	SendEvent(Message) error
	Close() error
}
