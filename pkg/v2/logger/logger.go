package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/simplefxn/goircd/pkg/v2/server/config"
)

type Logger struct {
	bootstrap *config.Bootstrap
}

type Option func(o *Logger)

func Config(cfg *config.Bootstrap) Option {
	return func(l *Logger) {
		l.bootstrap = cfg
	}
}

func NewLog(opts ...Option) (zerolog.Logger, error) {
	var multi zerolog.LevelWriter

	zerolog.TimestampFieldName = "timestamp"
	writers := []io.Writer{}
	log := &Logger{}

	for _, o := range opts {
		o(log)
	}

	var consoleWriter io.Writer
	if log.bootstrap.PrettyConsole {
		consoleWriter = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}

		writers = append(writers, consoleWriter)
	} else {
		writers = append(writers, os.Stdout)
	}

	multi = zerolog.MultiLevelWriter(writers...)

	hostname, _ := os.Hostname()

	return zerolog.New(multi).With().
		Timestamp().
		Caller().
		Str("host", hostname).
		Logger(), nil
}
