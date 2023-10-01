package task

import (
	"context"
	"testing"

	"eventmapper/pkg/config"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestCreate(t *testing.T) {
	_, err := New(
		Config(&config.Bootstrap{}),
		Logger(zerolog.DefaultContextLogger),
	)
	require.NoError(t, err)
}

func TestStart(t *testing.T) {
	mainContext, cancel := context.WithCancel(context.Background())
	eg, ctx := errgroup.WithContext(mainContext)

	task, err := New(
		Config(&config.Bootstrap{}),
		Logger(zerolog.DefaultContextLogger),
	)
	require.NoError(t, err)

	eg.Go(func() error {
		<-ctx.Done() // wait for stop signal
		stopCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		err := task.Stop(stopCtx)
		return err
	})

	eg.Go(func() error {
		return task.Start(ctx)
	})

	cancel()

	if err := eg.Wait(); err != nil {
		require.NoError(t, err)
	}
}
