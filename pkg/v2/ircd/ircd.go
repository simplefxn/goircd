package ircd

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/simplefxn/goircd/internal/pipeline"
	config "github.com/simplefxn/goircd/pkg/v2/config"

	"github.com/rs/zerolog"
)

type Server struct {
	config    *config.Bootstrap
	log       *zerolog.Logger
	listener  net.Listener
	pipe      pipeline.Pipeline
	stop      chan bool
	name      string
	isStarted bool
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

	var err error

	srv := &Server{
		stop: make(chan bool),
	}

	for _, o := range opts {
		o(srv)
	}

	if srv.config == nil {
		return nil, fmt.Errorf("cannot start translator without a configuration")
	}

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

	return srv, nil
}

func (s *Server) Start(ctx context.Context) error {
	s.isStarted = true
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	if s.isStarted {
		s.isStarted = false
	}

	return nil
}
