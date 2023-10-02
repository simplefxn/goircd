package service

import (
	"context"

	"github.com/simplefxn/goircd/internal/pipeline"
	config "github.com/simplefxn/goircd/pkg/v2/journal/config"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Service struct {
	config    *config.Journal
	log       *zerolog.Logger
	stop      chan bool
	pipe      pipeline.Pipeline
	name      string
	isStarted bool
}

type Option func(o *Service)

func Config(cfg *config.Journal) Option {
	return func(s *Service) { s.config = cfg }
}

func Logger(l *zerolog.Logger) Option {
	return func(s *Service) { s.log = l }
}

func Next(next pipeline.Pipeline) Option {
	return func(s *Service) { s.pipe = next }
}

func Name(name string) Option {
	return func(s *Service) { s.name = name }
}

func (s *Service) Name() string {
	return s.name
}

func New(opts ...Option) (*Service, error) {
	proc := &Service{
		stop: make(chan bool),
	}

	for _, o := range opts {
		o(proc)
	}

	if proc.config == nil {
		log.Fatal().Msg("cannot start basic task without a configuration")
	}

	return proc, nil
}

func (s *Service) Start(ctx context.Context) error {
	s.isStarted = true
	return nil
}

func (s *Service) Stop(ctx context.Context) error {
	if s.isStarted {
		s.isStarted = false
	}

	return nil
}
