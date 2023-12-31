package main

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/simplefxn/goircd/pkg/v2/logger"
	"github.com/simplefxn/goircd/pkg/v2/server/config"
	"github.com/simplefxn/goircd/pkg/v2/server/ircd"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"gopkg.in/yaml.v3"
)

var flags = []cli.Flag{
	altsrc.NewStringFlag(&cli.StringFlag{
		Name:        "hostname",
		Value:       "localhost",
		Usage:       "hostname of the irc server",
		Destination: &config.Get().Hostname,
	}),
	altsrc.NewStringFlag(&cli.StringFlag{
		Name:        "bind",
		Value:       ":6667",
		Usage:       "address to bind to",
		Destination: &config.Get().Bind,
	}),
	altsrc.NewStringFlag(&cli.StringFlag{
		Name:        "motd",
		Value:       "",
		Usage:       "path to motd file",
		Destination: &config.Get().Motd,
	}),
	altsrc.NewStringFlag(&cli.StringFlag{
		Name:        "sslKey",
		Value:       "",
		Usage:       "path to ssl key file",
		Destination: &config.Get().SSLKey,
	}),
	altsrc.NewStringFlag(&cli.StringFlag{
		Name:        "sslCert",
		Value:       "",
		Usage:       "path to ssl cert file",
		Destination: &config.Get().SSLCert,
	}),
	altsrc.NewStringFlag(&cli.StringFlag{
		Name:        "sslCA",
		Value:       "",
		Usage:       "path to ssl ca file",
		Destination: &config.Get().SSLCA,
	}),
	altsrc.NewBoolFlag(&cli.BoolFlag{
		Name:        "prettyConsole",
		Value:       false,
		Usage:       "log pretty messages in the console",
		Destination: &config.Get().PrettyConsole,
	}),
	&cli.StringFlag{
		Name:  "config",
		Usage: "config filename",
	},
}

func CmdRun() *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "run irc server",
		Action: func(cCtx *cli.Context) error {
			zerolog.DurationFieldUnit = time.Second

			lg, err := logger.NewLog(
				logger.Config(config.Get()),
			)

			if err != nil {
				return err
			}

			server, err := ircd.New(
				ircd.Config(config.Get()),
				ircd.Logger(&lg),
			)
			if err != nil {
				return err
			}

			configFile, err := os.ReadFile(cCtx.String("config"))
			if err != nil {
				return err
			}

			natsRooms := config.Nats{}

			err = yaml.Unmarshal(configFile, &natsRooms)
			if err != nil {
				return err
			}
			// Create channels for NATS
			for _, room := range natsRooms.Channels {
				server.RoomFortNats(room)
			}

			err = server.Start(cCtx.Context)
			if err != nil {
				return err
			}

			return nil
		},
		Before: altsrc.InitInputSourceWithContext(flags, altsrc.NewYamlSourceFromFlagFunc("config")),
		Flags:  flags,
	}
}
