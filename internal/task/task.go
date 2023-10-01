package task

import "context"

type Task interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
