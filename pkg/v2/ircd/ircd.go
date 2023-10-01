package ircd

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/simplefxn/goircd/internal/pipeline"
	"github.com/simplefxn/goircd/pkg/v2/client"
	config "github.com/simplefxn/goircd/pkg/v2/config"

	"github.com/rs/zerolog"
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

func Logger(log *zerolog.Logger) ServerOption {
	return func(s *Server) { s.log = log }
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

	var log zerolog.Logger

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
		log = srv.log.With().Str("task", "task").Logger()
	} else {
		log = srv.log.With().Str("task", srv.name).Logger()
	}

	srv.log = &log

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
			// case _ := <-s.events:
			//	s.CheckAliveness(ctx)
		}

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
