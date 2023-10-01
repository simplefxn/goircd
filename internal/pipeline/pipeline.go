package pipeline

type Message []byte

type Messages []Message

type Pipeline interface {
	Next(*Messages)
}

func (msg Message) String() string {
	return string(msg)
}
