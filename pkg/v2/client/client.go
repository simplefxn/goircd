package client

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/simplefxn/goircd/internal/pipeline"
	config "github.com/simplefxn/goircd/pkg/v2/config"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	CRLF           = "\x0d\x0a"
	BufSize        = 1380
	PingTimeout    = time.Second * 180 // Max time deadline for client's unresponsiveness
	PingThreashold = time.Second * 90  // Max idle client's time before PING are sent
)

type Client struct {
	timestamp  time.Time
	pipe       pipeline.Pipeline
	conn       net.Conn
	config     *config.Bootstrap
	log        *zerolog.Logger
	stop       chan bool
	events     chan Event
	name       string
	hostname   string
	Nickname   string
	Username   string
	Realname   string
	isStarted  bool
	pingSent   bool
	Registered bool
}

type Option func(o *Client)

func Config(cfg *config.Bootstrap) Option {
	return func(c *Client) { c.config = cfg }
}

func Logger(l *zerolog.Logger) Option {
	return func(c *Client) { c.log = l }
}

func Next(next pipeline.Pipeline) Option {
	return func(c *Client) { c.pipe = next }
}

func Name(name string) Option {
	return func(c *Client) { c.name = name }
}

func Hostname(host string) Option {
	return func(c *Client) { c.hostname = host }
}

func Connection(conn net.Conn) Option {
	return func(c *Client) { c.conn = conn }
}

func Events(ev chan Event) Option {
	return func(c *Client) { c.events = ev }
}

func (c *Client) Name() string {
	return c.name
}

func (c *Client) String() string {
	return c.Nickname + "!" + c.Username + "@" + c.conn.RemoteAddr().String()
}

func New(opts ...Option) (*Client, error) {
	var logger zerolog.Logger

	proc := &Client{
		stop: make(chan bool),
	}

	for _, o := range opts {
		o(proc)
	}

	if proc.name == "" {
		logger = proc.log.With().Str("task", "task").Logger()
	} else {
		logger = proc.log.With().Str("task", proc.name).Logger()
	}

	proc.log = &logger

	if proc.config == nil {
		return nil, fmt.Errorf("cannot start translator without a configuration")
	}

	if proc.events == nil {
		return nil, fmt.Errorf("cannot start without an event channel")
	}

	return proc, nil
}

func (c *Client) Start(ctx context.Context) error {
	var bufNet []byte

	c.isStarted = true
	buf := make([]byte, 0)

	for {
		bufNet = make([]byte, BufSize)

		_, err := c.conn.Read(bufNet)
		if err != nil {
			log.Err(err).Msg("connection lost")
		}

		c.timestamp = time.Now()
		c.pingSent = false
		bufNet = bytes.TrimRight(bufNet, "\x00")

		buf = append(buf, bufNet...)
		if !bytes.HasSuffix(buf, []byte(CRLF)) {
			continue
		}

		for _, msg := range bytes.Split(buf[:len(buf)-2], []byte(CRLF)) {
			if len(msg) > 0 {
				c.events <- Event{c, string(msg), EventMsg}
			}
		}

		buf = []byte{}
	}
}

func (c *Client) Stop(ctx context.Context) error {
	if c.isStarted {
		c.isStarted = false

		err := c.conn.Close()
		if err != nil {
			c.log.Err(err).Msg("closing connection")
		}
	}

	return nil
}

// Send message as is with CRLF appended.
func (c *Client) Msg(text string) error {
	_, err := c.conn.Write([]byte(text + CRLF))
	return err
}

// Send message from server. It has ": servername" prefix.
func (c *Client) Reply(text string) error {
	return c.Msg(":" + c.hostname + " " + text)
}

// Send server message, concatenating all provided text parts and
// prefix the last one with ":".
func (c *Client) ReplyParts(code string, text ...string) error {
	parts := []string{code}
	parts = append(parts, text...)
	parts[len(parts)-1] = ":" + parts[len(parts)-1]

	return c.Reply(strings.Join(parts, " "))
}

// Send nicknamed server message. After servername it always has target
// client's nickname. The last part is prefixed with ":".
func (c *Client) ReplyNicknamed(code string, text ...string) error {
	return c.ReplyParts(code, append([]string{c.Nickname}, text...)...)
}

// Reply "461 not enough parameters" error for given command.
func (c *Client) ReplyNotEnoughParameters(command string) error {
	return c.ReplyNicknamed("461", command, "Not enough parameters")
}

// Reply "403 no such channel" error for specified channel.
func (c *Client) ReplyNoChannel(channel string) error {
	return c.ReplyNicknamed("403", channel, "No such channel")
}

func (c *Client) ReplyNoNickChan(channel string) error {
	return c.ReplyNicknamed("401", channel, "No such nick/channel")
}

func (c *Client) SendPing(now time.Time) error {
	if c.timestamp.Add(PingTimeout).Before(now) {
		log.Info().Msg("ping timeout")
		return c.conn.Close()
	}

	if !c.pingSent && c.timestamp.Add(PingThreashold).Before(now) {
		if c.Registered {
			err := c.Msg("PING :" + c.hostname)
			if err != nil {
				log.Err(err).Msg("cannot send ping")
			}

			c.pingSent = true
		} else {
			log.Info().Msg("ping timeout")
			return c.conn.Close()
		}
	}

	return nil
}
