package skel

import (
	"context"
	"errors"
	"fmt"

	"eventmapper/internal/pipeline"
	config "eventmapper/pkg/config"

	"github.com/rs/zerolog"
)

var ErrTranslation = errors.New("error")

func ErrGenericError(text string) error {
	return fmt.Errorf("translate %w : %s", ErrTranslation, text)
}

func ErrGenericErrorWrap(text string, err error) error {
	return fmt.Errorf("translate %s : %w", text, err)
}

type Task struct {
	config *config.Bootstrap
	log    *zerolog.Logger
	pipe   pipeline.Pipeline
	name   string
}

type TaskOption func(o *Task)

func Config(config *config.Bootstrap) TaskOption {
	return func(t *Task) { t.config = config }
}

func Logger(log *zerolog.Logger) TaskOption {
	return func(t *Task) { t.log = log }
}

func Next(next pipeline.Pipeline) TaskOption {
	return func(t *Task) { t.pipe = next }
}

func Name(name string) TaskOption {
	return func(t *Task) { t.name = name }
}

func New(opts ...TaskOption) (*Task, error) {

	proc := &Task{}

	for _, o := range opts {
		o(proc)
	}

	if proc.config == nil {
		return nil, ErrGenericError("cannot start translator without a configuration")
	}

	return proc, nil
}

func (t *Task) Start(ctx context.Context) error {

	return nil
}

func (t *Task) Stop(ctx context.Context) error {

	return nil
}

func (t *Task) Name() string {

	return t.name
}
