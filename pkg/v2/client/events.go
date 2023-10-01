package client

import "fmt"

type EventType int

const (
	EventNew EventType = iota
	EventDel
	EventTopic
	EventWho
	EventMode
	EventMsg
)

type Event struct {
	Client    *Client
	Text      string
	EventType EventType
}

func (e EventType) String() string {
	switch e {
	case EventDel:
		return "DEL"
	case EventMode:
		return "MODE"
	case EventNew:
		return "NEW"
	case EventTopic:
		return "TOPIC"
	case EventWho:
		return "WHO"
	case EventMsg:
		return "MSG"
	default:
		return fmt.Sprintf("%d", int(e))
	}
}
