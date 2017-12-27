package main

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/codefresh-io/hermes/pkg/version"
)

func main() {
	app := cli.NewApp()
	app.Name = "hermes"
	app.Authors = []cli.Author{{Name: "Alexei Ledenev", Email: "alexei@codefresh.io"}}
	app.Version = version.HumanVersion
	app.EnableBashCompletion = true
	app.Usage = "configure triggers and run trigger manager server"
	app.UsageText = fmt.Sprintf(`Configure triggers for Codefresh pipeline execution or start trigger manager server. Process "normalized" events and run Codefresh pipelines with variables extracted from events payload.
%s
hermes respects following environment variables:
   - STORE_HOST         - set the url to the Redis store server (default localhost)
   - STORE_PORT         - set Redis store port (default to 6379)
   - STORE_PASSWORD     - set Redis store password
   
Copyright © Codefresh.io`, version.ASCIILogo)
	app.Before = before

	app.Commands = []cli.Command{
		serverCommand,
		triggerCommand,
		pipelineCommand,
	}
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "codefresh, cf",
			Usage:  "Codefresh API endpoint",
			Value:  "https://g.codefresh.io/",
			EnvVar: "CFAPI_URL",
		},
		cli.StringFlag{
			Name:   "token, t",
			Usage:  "Codefresh API token",
			EnvVar: "CFAPI_TOKEN",
		},
		cli.StringFlag{
			Name:   "redis",
			Usage:  "redis store host name",
			Value:  "localhost",
			EnvVar: "STORE_HOST",
		},
		cli.IntFlag{
			Name:   "redis-port",
			Usage:  "redis store port",
			Value:  6379,
			EnvVar: "STORE_PORT",
		},
		cli.StringFlag{
			Name:   "redis-password",
			Usage:  "redis store password",
			EnvVar: "STORE_PASSWORD",
		},
		cli.BoolFlag{
			Name:   "debug",
			Usage:  "enable debug mode with verbose logging",
			EnvVar: "DEBUG_HERMES",
		},
		cli.BoolFlag{
			Name:  "dry-run",
			Usage: "do not execute commands, just log",
		},
		cli.BoolFlag{
			Name:  "json",
			Usage: "produce log in JSON format: Logstash and Splunk friendly",
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func before(c *cli.Context) error {
	// set debug log level
	if c.GlobalBool("debug") {
		log.SetLevel(log.DebugLevel)
	}
	// set log formatter to JSON
	if c.GlobalBool("json") {
		log.SetFormatter(&log.JSONFormatter{})
	}

	return nil
}
