/*
goircd -- minimalistic simple Internet Relay Chat (IRC) server
Copyright (C) 2014 Sergey Matveev <stargrave@stargrave.org>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/
package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

var (
	hostname string
	bind     string
	motd     string

	sslKey        string
	sslCert       string
	sslConfigFile string
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
