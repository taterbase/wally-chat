package session

import "testing"

func createTelnet() *Telnet {
	return NewTelnet(nil, 5, "fuschia", "testchannel")
}

func TestTelnetMessageCreation(t *testing.T) {
	tel := createTelnet()
	msgText := "testerooni"
	msg := tel.newMessage([]byte(msgText))

	if msg.Channel != tel.Channel() {
		t.Errorf("incorrect channel name for message %s",
			msg.Channel)
	}

	if msg.From != tel {
		t.Errorf("incorrect session for message %v",
			msg.From)
	}

	if msg.Body != msgText {
		t.Errorf("incorrect content for message %v",
			msg.Body)
	}
}

func TestTelnetRemovesUserEscapeSequences(t *testing.T) {
	tel := createTelnet()
	msgText := "testerooni"
	msgWithEscape := "\033" + msgText
	msg := tel.newMessage([]byte(msgWithEscape))

	if msg.Body != msgText {
		t.Errorf("incorrect content for message %v",
			msg.Body)
	}
}
