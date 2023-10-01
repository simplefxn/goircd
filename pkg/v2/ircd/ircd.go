package ircd

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/simplefxn/goircd/internal/pipeline"
	"github.com/simplefxn/goircd/pkg/v2/client"
	config "github.com/simplefxn/goircd/pkg/v2/config"

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
			case client.EventTopic:
			case client.EventWho:
			default:
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
		loopErr := cli.ReplyNicknamed("372", "- "+string(str))
		if loopErr != nil {
			s.log.Err(loopErr).Msg("cannot send message")
		}
	}

	err = cli.ReplyNicknamed("376", "End of /MOTD command")
	if err != nil {
		s.log.Err(err).Msg("cannot send message")
	}
}
