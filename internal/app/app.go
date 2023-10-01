package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"sync"

	"github.com/simplefxn/goircd/internal/task"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

var ErrApp = errors.New("error")

func ErrGenericError(text string) error {
	return fmt.Errorf("application %w : %s", ErrApp, text)
}

func ErrGenericErrorWrap(text string, err error) error {
	return fmt.Errorf("application %w : %s", err, text)
}

type Info interface {
	ID() string
	Name() string
	Version() string
	Metadata() map[string]string
	Endpoint() []string
}

type App struct {
	ctx       context.Context
	cancel    func()
	id        string
	name      string
	version   string
	metadata  map[string]string
	endpoints []*url.URL

	tasks []task.Task
	log   *zerolog.Logger
	sigs  []os.Signal
}

type Option func(a *App)

// ID with service id.
func ID(id string) Option {
	return func(a *App) { a.id = id }
}

// Name with service name.
func Name(name string) Option {
	return func(a *App) { a.name = name }
}

// Version with service version.
func Version(version string) Option {
	return func(a *App) { a.version = version }
}

// Metadata with service metadata.
func Metadata(md map[string]string) Option {
	return func(a *App) { a.metadata = md }
}

// Endpoint with service endpoint.
func Endpoint(endpoints ...*url.URL) Option {
	return func(a *App) { a.endpoints = endpoints }
}

// Context with service context.
func Context(parentCtx context.Context) Option {
	ctx, cancel := context.WithCancel(parentCtx)

	return func(a *App) {
		a.ctx = ctx
		a.cancel = cancel
	}
}

// Signal with exit signals.
func Signal(sigs ...os.Signal) Option {
	return func(a *App) { a.sigs = sigs }
}

// Process to start.
func Task(tasks ...task.Task) Option {
	return func(a *App) { a.tasks = tasks }
}

// Signal with exit signals.
func Logger(log *zerolog.Logger) Option {
	return func(a *App) { a.log = log }
}

func New(opts ...Option) *App {
	a := App{}

	if id, err := uuid.NewUUID(); err == nil {
		a.id = id.String()
	}

	for _, opt := range opts {
		opt(&a)
	}

	log := a.log.With().Logger()
	a.log = &log

	return &a
}

// ID returns app instance id.
func (a *App) ID() string { return a.id }

// Name returns service name.
func (a *App) Name() string { return a.name }

// Version returns app version.
func (a *App) Version() string { return a.version }

// Metadata returns service metadata.
func (a *App) Metadata() map[string]string { return a.metadata }

// Endpoint returns endpoints.
func (a *App) Endpoint() []string {
	return []string{}
}

// Run executes all OnStart hooks registered with the application's Lifecycle.
func (a *App) Run() error {
	var err error

	eg, ctx := errgroup.WithContext(a.ctx)
	wg := sync.WaitGroup{}

	for _, task := range a.tasks {
		task := task

		// The first call to return a non-nil error cancels the group's context. The error will be returned by Wait.
		eg.Go(func() error {
			<-ctx.Done() // wait for stop signal
			stopCtx, cancel := context.WithCancel(a.ctx)
			defer cancel()
			err := task.Stop(stopCtx)

			zerolog.Dict().Str("task", task.Name())
			a.log.Info().Str("task", task.Name()).Dict("details", zerolog.Dict()).Msgf("stopped")

			if err != nil {
				return ErrGenericErrorWrap("stopping task", err)
			}

			return nil
		})

		wg.Add(1)
		eg.Go(func() error {
			wg.Done() // here is to ensure server start has begun running before register, so defer is not needed
			a.log.Info().Str("task", task.Name()).Dict("details", zerolog.Dict()).Msgf("starting")

			if err := task.Start(ctx); err != nil {

				return ErrGenericErrorWrap(fmt.Sprintf("starting task %s", task.Name()), err)
			}
			return nil
		})
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, a.sigs...)

	eg.Go(func() error {
		select {
		case <-ctx.Done():
			return nil
		case <-c:
			a.log.Info().Msg("received stop signal")
			return a.Stop()
		}
	})

	if err = eg.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		a.log.Error().Msg(err.Error())
		return ErrGenericErrorWrap("while waiting for context to be canceled", err)
	}

	return ErrGenericErrorWrap("cannot run", err)
}

func (a *App) Stop() error {

	if a.cancel != nil {
		a.cancel()
	}

	return nil
}
