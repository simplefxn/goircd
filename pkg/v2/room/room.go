package room

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/simplefxn/goircd/internal/pipeline"
	"github.com/simplefxn/goircd/pkg/v2/client"
	config "github.com/simplefxn/goircd/pkg/v2/config"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	ReRoom = regexp.MustCompile("^#[^\x00\x07\x0a\x0d ,:/]{1,200}$")
)

type Room struct {
	pipe      pipeline.Pipeline
	config    *config.Bootstrap
	log       *zerolog.Logger
	stop      chan bool
	members   map[*client.Client]bool
	events    chan client.Event
	Name      string
	topic     string
	Key       string
	hostname  string
	isStarted bool
}

type Option func(o *Room)

func Config(cfg *config.Bootstrap) Option {
	return func(r *Room) { r.config = cfg }
}

func Logger(l *zerolog.Logger) Option {
	return func(r *Room) { r.log = l }
}

func Next(next pipeline.Pipeline) Option {
	return func(r *Room) { r.pipe = next }
}

func Name(name string) Option {
	return func(r *Room) { r.Name = name }
}

func Topic(name string) Option {
	return func(r *Room) { r.topic = name }
}

func Key(name string) Option {
	return func(r *Room) { r.Key = name }
}

func Hostname(name string) Option {
	return func(r *Room) { r.hostname = name }
}

func Events(evs chan client.Event) Option {
	return func(r *Room) { r.events = evs }
}

func New(opts ...Option) (*Room, error) {
	proc := &Room{
		stop:    make(chan bool),
		members: make(map[*client.Client]bool),
	}

	for _, o := range opts {
		o(proc)
	}

	if proc.config == nil {
		log.Fatal().Msg("cannot start translator without a configuration")
	}

	return proc, nil
}

func (r *Room) Start(ctx context.Context) error {
	var cli *client.Client

	r.isStarted = true

	for {
		select {
		case <-r.stop:
			return nil
		case ev := <-r.events:
			switch ev.EventType {
			case client.EventNew:
				r.members[cli] = true

				r.SendTopic(cli)
				r.Broadcast(fmt.Sprintf(":%s JOIN %s", cli, r.Name))

				nicknames := []string{}
				for member := range r.members {
					nicknames = append(nicknames, member.Nickname)
				}

				sort.Strings(nicknames)

				err := cli.ReplyNicknamed("353", "=", r.Name, strings.Join(nicknames, " "))
				if err != nil {
					return err
				}

				err = cli.ReplyNicknamed("366", r.Name, "End of NAMES list")
				if err != nil {
					return err
				}

			case client.EventDel:
				if _, subscribed := r.members[cli]; !subscribed {
					err := cli.ReplyNicknamed("442", r.Name, "You are not on that channel")
					if err != nil {
						return err
					}

					continue
				}

				delete(r.members, cli)

				msg := fmt.Sprintf(":%s PART %s :%s", cli, r.Name, cli.Nickname)

				go r.Broadcast(msg)

			case client.EventTopic:
				if _, subscribed := r.members[cli]; !subscribed {
					err := cli.ReplyParts("442", r.Name, "You are not on that channel")
					if err != nil {
						return err
					}

					continue
				}

				if ev.Text == "" {
					go r.SendTopic(cli)

					continue
				}

				r.topic = strings.TrimLeft(ev.Text, ":")

				msg := fmt.Sprintf(":%s TOPIC %s :%s", cli, r.Name, r.topic)
				go r.Broadcast(msg)

			case client.EventWho:
				for m := range r.members {
					err := cli.ReplyNicknamed("352", r.Name, m.Username, m.RemoteHost, r.hostname, m.Nickname, "H", "0 "+m.Realname)
					if err != nil {
						return err
					}
				}

				err := cli.ReplyNicknamed("315", r.Name, "End of /WHO list")
				if err != nil {
					return err
				}

			case client.EventMode:
				if ev.Text == "" {
					mode := "+"
					if r.Key != "" {
						mode += "k"
					}

					err := cli.Msg(fmt.Sprintf("324 %s %s %s", cli.Nickname, r.Name, mode))
					if err != nil {
						return err
					}

					continue
				}

				if strings.HasPrefix(ev.Text, "-k") || strings.HasPrefix(ev.Text, "+k") {
					if _, subscribed := r.members[cli]; !subscribed {
						err := cli.ReplyParts("442", r.Name, "You are not on that channel")
						if err != nil {
							return err
						}

						continue
					}
				} else {
					err := cli.ReplyNicknamed("472", ev.Text, "Unknown MODE flag")

					if err != nil {
						return err
					}

					continue
				}

				var msg string

				if strings.HasPrefix(ev.Text, "+k") {
					cols := strings.Split(ev.Text, " ")
					if len(cols) == 1 {
						err := cli.ReplyNotEnoughParameters("MODE")
						if err != nil {
							return err
						}

						continue
					}

					r.Key = cols[1]
					msg = fmt.Sprintf(":%s MODE %s +k %s", cli, r.Name, r.Key)
				} else if strings.HasPrefix(ev.Text, "-k") {
					r.Key = ""
					msg = fmt.Sprintf(":%s MODE %s -k", cli, r.Name)
				}

				go r.Broadcast(msg)

			case client.EventMsg:
				sep := strings.Index(ev.Text, " ")

				r.Broadcast(fmt.Sprintf(":%s %s %s :%s", cli, ev.Text[:sep], r.Name, ev.Text[sep+1:]), cli)
			}
		}
	}
}

func (r *Room) Stop(ctx context.Context) error {
	if r.isStarted {
		r.isStarted = false
		r.stop <- true
	}

	return nil
}

func (r *Room) SendTopic(cli *client.Client) {
	if r.topic == "" {
		err := cli.ReplyNicknamed("331", r.Name, "No topic is set")
		if err != nil {
			r.log.Err(err).Msg("cannot send message")
		}
	} else {
		err := cli.ReplyNicknamed("332", r.Name, r.topic)
		if err != nil {
			r.log.Err(err).Msg("cannot send message")
		}
	}
}

func (r *Room) Broadcast(msg string, clientToIgnore ...*client.Client) {
	for member := range r.members {
		if (len(clientToIgnore) > 0) && member == clientToIgnore[0] {
			continue
		}

		err := member.Msg(msg)
		if err != nil {
			r.log.Err(err).Msg("cannot send message")
		}
	}
}

// Sanitize room's name. It can consist of 1 to 50 ASCII symbols
// with some exclusions. All room names will have "#" prefix.
func NameValid(name string) bool {
	return ReRoom.MatchString(name)
}
