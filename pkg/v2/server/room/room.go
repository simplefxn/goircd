package room

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/simplefxn/goircd/internal/pipeline"
	"github.com/simplefxn/goircd/pkg/v2/server/client"
	config "github.com/simplefxn/goircd/pkg/v2/server/config"

	"github.com/rs/zerolog"
)

var (
	ReRoom = regexp.MustCompile("^#[^\x00\x07\x0a\x0d ,:/]{1,200}$")
)

type Room struct {
	pipe       pipeline.Pipeline
	config     *config.Bootstrap
	log        *zerolog.Logger
	stop       chan bool
	Members    map[*client.Client]bool
	events     chan client.Event
	Name       string
	Topic      string
	Key        string
	hostname   string
	isStarted  bool
	natsConfig *config.NatsChannel
	nc         *nats.Conn
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
	return func(r *Room) { r.Topic = name }
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

func Nats(nts *config.NatsChannel) Option {
	return func(r *Room) { r.natsConfig = nts }
}

func New(opts ...Option) (*Room, error) {

	var err error

	proc := &Room{
		stop:    make(chan bool),
		Members: make(map[*client.Client]bool),
	}

	for _, o := range opts {
		o(proc)
	}

	if proc.config == nil {
		return nil, fmt.Errorf("cannot start room without a configuration")
	}

	if proc.natsConfig != nil {
		proc.nc, err = nats.Connect(proc.natsConfig.URL)
		if err != nil {
			return nil, err
		}

		if (*proc.natsConfig).Direction == "input" {
			proc.nc.Subscribe(proc.natsConfig.Name, func(msg *nats.Msg) {
				proc.Broadcast(string(msg.Data))
			})
		}
		if proc.natsConfig.Topic != "" {
			proc.Topic = proc.natsConfig.Topic
		}
	}

	return proc, nil
}

func (r *Room) Start(ctx context.Context) error {
	var cli *client.Client

	r.isStarted = true
	r.log.Info().Dict("details", zerolog.Dict().Str("name", r.Name)).Msg("started")

	r.log.Debug().Msgf("room ch %v", r.events)

	for {
		select {
		case <-r.stop:
			return nil
		case ev := <-r.events:
			cli = ev.Client

			r.log.Debug().Dict("details",
				zerolog.Dict().
					Str("type", ev.EventType.String()).
					Str("text", ev.Text).
					Str("remote", ev.Client.RemoteHost),
			).Msg("room received event")

			switch ev.EventType {
			case client.EventNew:
				r.Members[cli] = true

				r.SendTopic(cli)
				r.Broadcast(fmt.Sprintf(":%s JOIN %s", cli, r.Name))

				nicknames := []string{}
				for member := range r.Members {
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
				if _, subscribed := r.Members[cli]; !subscribed {
					err := cli.ReplyNicknamed("442", r.Name, "You are not on that channel")
					if err != nil {
						return err
					}

					continue
				}

				delete(r.Members, cli)

				msg := fmt.Sprintf(":%s PART %s :%s", cli, r.Name, cli.Nickname)

				go r.Broadcast(msg)

			case client.EventTopic:
				if _, subscribed := r.Members[cli]; !subscribed {
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

				r.Topic = strings.TrimLeft(ev.Text, ":")

				msg := fmt.Sprintf(":%s TOPIC %s :%s", cli, r.Name, r.Topic)
				go r.Broadcast(msg)

			case client.EventWho:
				for m := range r.Members {
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
					if _, subscribed := r.Members[cli]; !subscribed {
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
				r.log.Info().Dict("details", zerolog.Dict().Str("client", cli.RemoteHost)).Msg(ev.Text)
				r.Broadcast(fmt.Sprintf(":%s %s %s :%s", cli, ev.Text[:sep], r.Name, ev.Text[sep+1:]), cli)

				if r.nc != nil {
					r.nc.Publish(r.Name, []byte(ev.Text[sep+1:]))
				}
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
	if r.Topic == "" {
		err := cli.ReplyNicknamed("331", r.Name, "No Topic is set")
		if err != nil {
			r.log.Err(err).Msg("cannot send message")
		}
	} else {
		err := cli.ReplyNicknamed("332", r.Name, r.Topic)
		if err != nil {
			r.log.Err(err).Msg("cannot send message")
		}
	}
}

func (r *Room) Broadcast(msg string, clientToIgnore ...*client.Client) {
	for member := range r.Members {
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
