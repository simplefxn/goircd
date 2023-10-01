package task

import (
	"context"

	"github.com/simplefxn/goircd/internal/pipeline"
	config "github.com/simplefxn/goircd/pkg/v2/config"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Basic struct {
	config    *config.Bootstrap
	log       *zerolog.Logger
	stop      chan bool
	pipe      pipeline.Pipeline
	name      string
	isStarted bool
}

type BasicOption func(o *Basic)

func Config(cfg *config.Bootstrap) BasicOption {
	return func(b *Basic) { b.config = cfg }
}

func Logger(l *zerolog.Logger) BasicOption {
	return func(b *Basic) { b.log = l }
}

func Next(next pipeline.Pipeline) BasicOption {
	return func(b *Basic) { b.pipe = next }
}

func Name(name string) BasicOption {
	return func(b *Basic) { b.name = name }
}

func (b *Basic) Name() string {
	return b.name
}

func New(opts ...BasicOption) (*Basic, error) {
	proc := &Basic{
		stop: make(chan bool),
	}

	for _, o := range opts {
		o(proc)
	}

	if proc.config == nil {
		log.Fatal().Msg("cannot start translator without a configuration")
	}

	return proc, nil
}

func (b *Basic) Start(ctx context.Context) error {
	b.isStarted = true
	return nil
}

func (b *Basic) Stop(ctx context.Context) error {
	if b.isStarted {
		b.isStarted = false
	}

	return nil
}
