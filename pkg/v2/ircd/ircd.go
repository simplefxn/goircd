package ircd

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/simplefxn/goircd/internal/pipeline"
	"github.com/simplefxn/goircd/pkg/v2/client"
	config "github.com/simplefxn/goircd/pkg/v2/config"
	"github.com/simplefxn/goircd/pkg/v2/room"

	"github.com/rs/zerolog"
)

var (
	ReNickname = regexp.MustCompile("^[a-zA-Z0-9-]{1,9}$")
)

const (
	PingTimeout    = time.Second * 180 // Max time deadline for client's unresponsiveness
	PingThreshold  = time.Second * 90  // Max idle client's time before PING are sent
	AlivenessCheck = time.Second * 10  // Client's aliveness check period
)

type Server struct {
	lastAlivenessCheck time.Time
	listener           net.Listener
	pipe               pipeline.Pipeline
	config             *config.Bootstrap
	log                *zerolog.Logger
	stop               chan bool
	events             chan client.Event
	clients            map[*client.Client]bool
	rooms              map[string]*room.Room
	roomCh             map[*room.Room]chan client.Event
	name               string
	isStarted          bool
}

type ServerOption func(o *Server)

func Config(cfg *config.Bootstrap) ServerOption {
	return func(s *Server) { s.config = cfg }
}

func Logger(logger *zerolog.Logger) ServerOption {
	return func(s *Server) { s.log = logger }
}

func Next(next pipeline.Pipeline) ServerOption {
	return func(s *Server) { s.pipe = next }
}

func Name(name string) ServerOption {
	return func(s *Server) { s.name = name }
}

func (s *Server) Name() string {
	return s.name
}

func New(opts ...ServerOption) (*Server, error) {
	var listener net.Listener

	var logger zerolog.Logger

	var err error

	srv := &Server{
		stop:    make(chan bool),
		events:  make(chan client.Event),
		clients: make(map[*client.Client]bool),
	}

	for _, o := range opts {
		o(srv)
	}

	if srv.config == nil {
		return nil, fmt.Errorf("cannot start translator without a configuration")
	}

	if srv.name == "" {
		logger = srv.log.With().Str("task", "task").Logger()
	} else {
		logger = srv.log.With().Str("task", srv.name).Logger()
	}

	srv.log = &logger

	tlsConfig := config.Get().GetServerConfig()
	if tlsConfig != nil {
		listener, err = tls.Listen("tcp", srv.config.Bind, tlsConfig)
		if err != nil {
			return nil, err
		}
	} else {
		listener, err = net.Listen("tcp", srv.config.Bind)
		if err != nil {
			return nil, err
		}
	}

	srv.listener = listener

	hostname, _ := os.Hostname()
	srv.config.Hostname = hostname

	return srv, nil
}

func (s *Server) Start(ctx context.Context) error {
	s.isStarted = true

	go s.handleNewConnection(ctx)

	for {
		select {
		case <-s.stop:
			err := s.Stop(ctx)
			return err
		case ev := <-s.events:
			s.CheckAliveness(ctx)
			s.lastAlivenessCheck = time.Now()

			cli := ev.Client

			switch ev.EventType {
			case client.EventNew:
				s.clients[cli] = true

			case client.EventDel:
				delete(s.clients, cli)
				// Forward event to room
				/*
						for _, room_sink := range daemon.room_sinks {
						room_sink <- event
					}
				*/
			case client.EventMode:
			case client.EventMsg:
				cols := strings.SplitN(ev.Text, " ", 2)
				command := strings.ToUpper(cols[0])

				if command == "QUIT" {
					delete(s.clients, cli)

					err := cli.Stop(ctx)
					if err != nil {
						s.log.Err(err).Msg("cannot send message")
					}

					continue
				}

				if !cli.Registered {
					go s.ClientRegister(cli, command, cols)

					continue
				}

				switch command {
				case "AWAY":
					continue
				case "JOIN":
					if len(cols) == 1 || len(cols[1]) < 1 {
						err := cli.ReplyNotEnoughParameters("JOIN")
						if err != nil {
							s.log.Err(err).Msg("cannot send message")
						}

						continue
					}

					go s.HandlerJoin(cli, cols[1])

				case "LIST":
					s.SendList(cli, cols)

				case "LUSERS":
					go s.SendLusers(cli)

				case "MODE":
					if len(cols) == 1 || len(cols[1]) < 1 {
						err := cli.ReplyNotEnoughParameters("MODE")
						if err != nil {
							return err
						}

						continue
					}

					cols = strings.SplitN(cols[1], " ", 2)
					if cols[0] == cli.Username {
						if len(cols) == 1 {
							err := cli.Msg("221 " + cli.Nickname + " +")
							if err != nil {
								return err
							}
						} else {
							err := cli.ReplyNicknamed("501", "Unknown MODE flag")
							if err != nil {
								return err
							}
						}

						continue
					}

					rm := cols[0]

					r, found := s.rooms[rm]
					if !found {
						err := cli.ReplyNoChannel(rm)
						if err != nil {
							return err
						}

						continue
					}

					if len(cols) == 1 {
						s.roomCh[r] <- client.Event{
							Client:    cli,
							Text:      "",
							EventType: client.EventMode}
					} else {
						s.roomCh[r] <- client.Event{
							Client:    cli,
							Text:      cols[1],
							EventType: client.EventMode}
					}

				case "MOTD":
					go s.SendMotd(cli)

				case "PART":
					if len(cols) == 1 || len(cols[1]) < 1 {
						err := cli.ReplyNotEnoughParameters("PART")

						if err != nil {
							return nil
						}

						continue
					}

					for _, rm := range strings.Split(cols[1], ",") {
						r, found := s.rooms[rm]
						if !found {
							err := cli.ReplyNoChannel(rm)
							if err != nil {
								return err
							}

							continue
						}

						s.roomCh[r] <- client.Event{
							Client:    cli,
							Text:      "",
							EventType: client.EventDel,
						}
					}

				case "PING":
					if len(cols) == 1 {
						err := cli.ReplyNicknamed("409", "No origin specified")
						if err != nil {
							return err
						}

						continue
					}

					err := cli.Reply(fmt.Sprintf("PONG %s :%s", s.config.Hostname, cols[1]))
					if err != nil {
						return err
					}

				case "PONG":
					continue

				case "NOTICE", "PRIVMSG":
					if len(cols) == 1 {
						err := cli.ReplyNicknamed("411", "No recipient given ("+command+")")
						if err != nil {
							return err
						}

						continue
					}

					cols = strings.SplitN(cols[1], " ", 2)
					if len(cols) == 1 {
						err := cli.ReplyNicknamed("412", "No text to send")
						if err != nil {
							return err
						}

						continue
					}

					msg := ""

					target := strings.ToLower(cols[0])
					for c := range s.clients {
						if c.Nickname == target {
							msg = fmt.Sprintf(":%s %s %s :%s", cli, command, c.Nickname, cols[1])

							err := c.Msg(msg)
							if err != nil {
								return err
							}

							break
						}
					}

					if msg != "" {
						continue
					}

					r, found := s.rooms[target]
					if !found {
						err := cli.ReplyNoNickChan(target)
						if err != nil {
							return err
						}
					}

					s.roomCh[r] <- client.Event{
						Client:    cli,
						EventType: client.EventMsg,
						Text:      command + " " + strings.TrimLeft(cols[1], ":"),
					}
				case "TOPIC":
					if len(cols) == 1 {
						err := cli.ReplyNotEnoughParameters("TOPIC")
						if err != nil {
							return err
						}

						continue
					}

					cols = strings.SplitN(cols[1], " ", 2)

					r, found := s.rooms[cols[0]]
					if !found {
						err := cli.ReplyNoChannel(cols[0])
						if err != nil {
							return err
						}

						continue
					}

					var change string

					if len(cols) > 1 {
						change = cols[1]
					} else {
						change = ""
					}

					s.roomCh[r] <- client.Event{
						Client:    cli,
						EventType: client.EventTopic,
						Text:      change,
					}
				case "WHO":
					if len(cols) == 1 || len(cols[1]) < 1 {
						err := cli.ReplyNotEnoughParameters("WHO")
						if err != nil {
							return err
						}

						continue
					}

					rm := strings.Split(cols[1], " ")[0]

					r, found := s.rooms[rm]
					if !found {
						err := cli.ReplyNoChannel(rm)
						if err != nil {
							return err
						}

						continue
					}
					s.roomCh[r] <- client.Event{
						Client:    cli,
						EventType: client.EventWho,
						Text:      "",
					}

				case "WHOIS":
					if len(cols) == 1 || len(cols[1]) < 1 {
						err := cli.ReplyNotEnoughParameters("WHOIS")
						if err != nil {
							return err
						}

						continue
					}

					cs := strings.Split(cols[1], " ")

					nicknames := strings.Split(cs[len(cs)-1], ",")
					go s.SendWhois(cli, nicknames)
				default:
					err := cli.ReplyNicknamed("421", command, "Unknown command")
					if err != nil {
						return err
					}
				}
			}
		}
	}
}

func (s *Server) ClientRegister(cli *client.Client, command string, cols []string) {
	switch command {
	case "NICK":
		if len(cols) == 1 || len(cols[1]) < 1 {
			err := cli.ReplyParts("431", "No nickname given")
			if err != nil {
				s.log.Err(err).Msg("cannot send message")
			}

			return
		}

		nickname := cols[1]
		for loopClient := range s.clients {
			if loopClient.Nickname == nickname {
				err := cli.ReplyParts("433", "*", nickname, "Nickname is already in use")
				if err != nil {
					s.log.Err(err).Msg("cannot send message")
				}

				return
			}
		}

		if !ReNickname.MatchString(nickname) {
			err := cli.ReplyParts("432", "*", cols[1], "Erroneous nickname")
			if err != nil {
				s.log.Err(err).Msg("cannot send message")
			}

			return
		}

		cli.Nickname = nickname

	case "USER":
		if len(cols) == 1 {
			err := cli.ReplyNotEnoughParameters("USER")
			if err != nil {
				s.log.Err(err).Msg("cannot send message")
			}

			return
		}

		args := strings.SplitN(cols[1], " ", 4)

		if len(args) < 4 {
			err := cli.ReplyNotEnoughParameters("USER")
			if err != nil {
				s.log.Err(err).Msg("cannot send message")
			}

			return
		}

		cli.Username = args[0]
		cli.Realname = strings.TrimLeft(args[3], ":")
	}

	if cli.Nickname != "*" && cli.Username != "" {
		var err error

		cli.Registered = true

		err = cli.ReplyNicknamed("001", "Hi, welcome to IRC")
		if err != nil {
			s.log.Err(err).Msg("cannot send message")
		}

		err = cli.ReplyNicknamed("002", "Your host is "+s.config.Hostname+", running goircd")
		if err != nil {
			s.log.Err(err).Msg("cannot send message")
		}

		err = cli.ReplyNicknamed("003", "This server was created sometime")
		if err != nil {
			s.log.Err(err).Msg("cannot send message")
		}

		err = cli.ReplyNicknamed("004", s.config.Hostname+" goircd o o")
		if err != nil {
			s.log.Err(err).Msg("cannot send message")
		}

		s.SendLusers(cli)
		s.SendMotd(cli)
	}
}

func (s *Server) CheckAliveness(ctx context.Context) {
	now := time.Now()

	if s.lastAlivenessCheck.Add(AlivenessCheck).Before(now) {
		for c := range s.clients {
			err := c.SendPing(now)
			if err != nil {
				s.log.Err(err).Msg("sending ping")
			}
		}

		s.lastAlivenessCheck = now
	}
}

func (s *Server) Stop(ctx context.Context) error {
	if s.isStarted {
		s.isStarted = false
		s.log.Info().Msg("stopped")
	}

	return nil
}

func (s *Server) handleNewConnection(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil { // we cannot accept more connections, should exit daemon or restart
			return
		}

		remoteHost := conn.RemoteAddr().String()

		cli, err := client.New(
			client.Hostname(s.config.Hostname),
			client.Name(remoteHost),
			client.Connection(conn),
			client.Events(s.events),
		)
		if err != nil {
			continue
		}

		err = cli.Start(ctx)
		if err != nil {
			return
		}
	}
}

func (s *Server) SendLusers(cli *client.Client) {
	lusers := 0

	for tmpCli := range s.clients {
		if tmpCli.Registered {
			lusers++
		}
	}

	err := cli.ReplyNicknamed("251", fmt.Sprintf("There are %d users and 0 invisible on 1 servers", lusers))
	if err != nil {
		s.log.Err(err).Msg("cannot send message")
	}
}

func (s *Server) SendMotd(cli *client.Client) {
	if s.config.Motd == "" {
		err := cli.ReplyNicknamed("422", "MOTD File is missing")
		if err != nil {
			s.log.Err(err).Msg("cannot send message")
		}

		return
	}

	motd, err := os.ReadFile(s.config.Motd)
	if err != nil {
		s.log.Err(err).Msgf("Can not read motd file %s", s.config.Motd)

		err = cli.ReplyNicknamed("422", "Error reading MOTD File")
		if err != nil {
			s.log.Err(err).Msg("cannot send message")
		}

		return
	}

	err = cli.ReplyNicknamed("375", "- "+s.config.Hostname+" Message of the day -")
	if err != nil {
		s.log.Err(err).Msg("cannot send message")
	}

	for _, str := range strings.Split(strings.Trim(string(motd), "\n"), "\n") {
		loopErr := cli.ReplyNicknamed("372", "- "+str)
		if loopErr != nil {
			s.log.Err(loopErr).Msg("cannot send message")
		}
	}

	err = cli.ReplyNicknamed("376", "End of /MOTD command")
	if err != nil {
		s.log.Err(err).Msg("cannot send message")
	}
}

func (s *Server) HandlerJoin(cli *client.Client, cmd string) {
	var keys []string

	args := strings.Split(cmd, " ")
	rooms := strings.Split(args[0], ",")

	if len(args) > 1 {
		keys = strings.Split(args[1], ",")
	} else {
		keys = []string{}
	}

	for n, r := range rooms {
		if !room.NameValid(r) {
			err := cli.ReplyNoChannel(r)
			if err != nil {
				s.log.Err(err).Msg("cannot send command")
			}

			continue
		}

		var key string

		if (n < len(keys)) && (keys[n] != "") {
			key = keys[n]
		} else {
			key = ""
		}

		denied := false
		joined := false

		for existingRoom, roomCh := range s.roomCh {
			if r == existingRoom.Name {
				if (existingRoom.Key != "") && (existingRoom.Key != key) {
					denied = true
				} else {
					roomCh <- client.Event{
						Client:    cli,
						Text:      "",
						EventType: client.EventNew,
					}
					joined = true
				}

				break
			}
		}

		if denied {
			err := cli.ReplyNicknamed("475", r, "Cannot join channel (+k) - bad key")
			if err != nil {
				s.log.Err(err).Msg("cannot send message")
			}
		}

		if denied || joined {
			continue
		}

		newRoom, roomCh := s.RoomRegister(r)
		if key != "" {
			newRoom.Key = key
		}

		roomCh <- client.Event{
			Client:    cli,
			Text:      "",
			EventType: client.EventNew,
		}
	}
}

// Register new room in Daemon. Create an object, events sink, save pointers
// to corresponding daemon's places and start room's processor goroutine.
func (s *Server) RoomRegister(name string) (newRoom *room.Room, roomCh chan client.Event) {
	newRoom, _ = room.New(
		room.Hostname(s.config.Hostname),
		room.Name(name),
	)

	roomCh = make(chan client.Event)

	s.rooms[name] = newRoom
	s.roomCh[newRoom] = roomCh

	go newRoom.Start(context.Background())

	return newRoom, roomCh
}

func (s *Server) SendList(cli *client.Client, cols []string) {
	var rooms []string

	if (len(cols) > 1) && (cols[1] != "") {
		rooms = strings.Split(strings.Split(cols[1], " ")[0], ",")
	} else {
		rooms = []string{}
		for rm := range s.rooms {
			rooms = append(rooms, rm)
		}
	}

	sort.Strings(rooms)

	for _, room := range rooms {
		r, found := s.rooms[room]
		if found {
			err := cli.ReplyNicknamed("322", room, fmt.Sprintf("%d", len(r.Members)), r.Topic)
			if err != nil {
				s.log.Err(err).Msg("cannot send message")
			}
		}
	}

	err := cli.ReplyNicknamed("323", "End of /LIST")
	if err != nil {
		s.log.Err(err).Msg("cannot send command")
	}
}

func (s *Server) SendWhois(cli *client.Client, nicknames []string) {
	for _, nickname := range nicknames {
		nickname = strings.ToLower(nickname)

		found := false

		for c := range s.clients {
			if !strings.EqualFold(c.Nickname, nickname) {
				continue
			}

			found = true
			h := c.RemoteHost

			h, _, err := net.SplitHostPort(h)
			if err != nil {
				log.Printf("Can't parse RemoteAddr %q: %v", h, err)
				h = "Unknown"
			}

			err = cli.ReplyNicknamed("311", c.Nickname, c.Username, h, "*", c.Realname)
			if err != nil {
				s.log.Err(err).Msg("cannot send command")
			}

			err = cli.ReplyNicknamed("312", c.Nickname, s.config.Hostname, s.config.Hostname)
			if err != nil {
				s.log.Err(err).Msg("cannot send command")
			}

			subscriptions := []string{}

			for _, room := range s.rooms {
				for subscriber := range room.Members {
					if subscriber.Nickname == nickname {
						subscriptions = append(subscriptions, room.Name)
					}
				}
			}

			sort.Strings(subscriptions)

			err = cli.ReplyNicknamed("319", c.Nickname, strings.Join(subscriptions, " "))
			if err != nil {
				s.log.Err(err).Msg("cannot send command")
			}

			err = cli.ReplyNicknamed("318", c.Nickname, "End of /WHOIS list")
			if err != nil {
				s.log.Err(err).Msg("cannot send command")
			}
		}

		if !found {
			err := cli.ReplyNoNickChan(nickname)
			if err != nil {
				s.log.Err(err).Msg("cannot send command")
			}
		}
	}
}
