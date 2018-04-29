package session

import "time"

// json deocoding/encoding supported even though we dont' use it
type Message struct {
	T       time.Time `json:"timestamp"`
	From    Session   `json:"session"`
	Body    string    `json:"body"`
	Channel string    `json:"channel"`
}

// helper method to generate message
func NewMessage(body, channel string, from Session) Message {
	return Message{
		Body:    body,
		From:    from,
		T:       time.Now(),
		Channel: channel,
	}
}
