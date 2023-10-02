package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "goircd",
		Usage: "minimalist irc server",
		Commands: []*cli.Command{
			CmdRun(),
			CmdCAGenerate(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
