package session

import "time"

type Message struct {
	T       time.Time
	From    Session
	Body    string
	Channel string
}

func NewMessage(body, channel string, from Session) Message {
	return Message{
		Body:    body,
		From:    from,
		T:       time.Now(),
		Channel: channel,
	}
}
