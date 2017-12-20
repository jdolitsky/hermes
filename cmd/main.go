package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/codefresh-io/hermes/pkg/model"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/codefresh-io/hermes/pkg/backend"
	"github.com/codefresh-io/hermes/pkg/codefresh"
	"github.com/codefresh-io/hermes/pkg/controller"
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
   - REDIS_HOST         - set the url to the Redis server (default localhost)
   - REDIS_PORT         - set Redis port (default to 6379)
   - REDIS_PASSWORD     - set Redis password
   
Copyright © Codefresh.io`, version.ASCIILogo)
	app.Before = before

	app.Commands = []cli.Command{
		{
			Name: "server",
			Flags: []cli.Flag{
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
				cli.IntFlag{
					Name:  "port",
					Usage: "TCP port for the trigger manager server",
					Value: 8080,
				},
			},
			Usage:       "start trigger manager server",
			Description: "Run Codefresh trigger manager server. Use REST API to manage triggers. Send normalized event payload to trigger endpoint to invoke associated Codefresh pipelines.",
			Action:      runServer,
		},
		{
			Name:  "trigger",
			Usage: "configure Codefresh triggers",
			Subcommands: []cli.Command{
				{
					Name: "get",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "filter, f",
							Usage: "trigger filter",
						},
						cli.StringFlag{
							Name:  "pipeline, p",
							Usage: "additional filter by pipeline URI",
						},
						cli.BoolFlag{
							Name:  "quiet, q",
							Usage: "only display event URIs",
						},
					},
					Usage:       "get trigger(s)",
					ArgsUsage:   "[name, filter or empty (ALL)]",
					Description: "Get trigger by name or filter, or get all triggers, if no filter specified",
					Action:      getTriggers,
				},
				{
					Name: "add",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "secret, s",
							Usage: "trigger secret (auto-generated if skipped)",
							Value: model.GenerateKeyword,
						},
					},
					Usage:       "add trigger",
					ArgsUsage:   "[event URI] [pipeline repo-owner] [pipeline repo-name] [pipeline name]",
					Description: "Add a new trigger connected to specified pipeline",
					Action:      addTrigger,
				},
				{
					Name:        "delete",
					Usage:       "delete trigger",
					ArgsUsage:   "[event URI]",
					Description: "Delete trigger by event URI",
					Action:      deleteTrigger,
				},
				{
					Name: "test",
					Flags: []cli.Flag{
						cli.StringSliceFlag{
							Name:  "var",
							Usage: "variable pairs (key=val); can pass multiple pairs",
						},
					},
					Usage:       "trigger pipeline execution with variables",
					ArgsUsage:   "[name]",
					Description: "Invoke trigger, specified by trigger name. Can pass multiple variable pairs (key=value), using --var flags.",
					Action:      testTrigger,
				},
			},
		},
		{
			Name:  "pipeline",
			Usage: "configure Codefresh trigger pipelines",
			Subcommands: []cli.Command{
				{
					Name:        "get",
					Usage:       "get pipelines connected to trigger",
					ArgsUsage:   "[event URI]",
					Description: "Get all pipelines connected to trigger with provided event URI",
					Action:      getTriggerPipelines,
				},
				{
					Name:        "add",
					Usage:       "add pipelines to existing trigger",
					ArgsUsage:   "[event URI] [pipeline repo-owner] [pipeline repo-name] [pipeline name] ",
					Description: "Add pipeline to existing trigger with specified event URI",
					Action:      addTriggerPipelines,
				},
				{
					Name:        "delete",
					Usage:       "delete pipeline from existing trigger",
					ArgsUsage:   "[event URI] [pipeline repo-owner] [pipeline repo-name] [pipeline name] ",
					Description: "Delete pipeline from existing trigger with specified event URI",
					Action:      deleteTriggerPipeline,
				},
			},
		},
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
			Usage:  "redis host name",
			Value:  "localhost",
			EnvVar: "REDIS_HOST",
		},
		cli.IntFlag{
			Name:   "redis-port",
			Usage:  "redis host port",
			Value:  6379,
			EnvVar: "REDIS_PORT",
		},
		cli.StringFlag{
			Name:   "redis-password",
			Usage:  "redis password",
			EnvVar: "REDIS_PASSWORD",
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

// start trigger manager server
func runServer(c *cli.Context) error {
	fmt.Println()
	fmt.Println(version.ASCIILogo)
	router := gin.Default()

	// get codefresh endpoint
	codefreshService := codefresh.NewCodefreshEndpoint(c.String("cf"), c.String("t"))

	triggerController := controller.NewController(backend.NewRedisStore(c.GlobalString("redis"), c.GlobalInt("redis-port"), c.GlobalString("redis-password"), codefreshService))

	// trigger management API
	router.Handle("GET", "/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/triggers")
	})
	router.Handle("GET", "/triggers/", triggerController.List) // pass filter as query parameter
	// TODO: implement
	// router.Handle("GET", "/triggers/types", triggerController.ListTypes)
	// router.Handle("GET", "/triggers/types/:type", triggerController.GetType)
	// router.Handle("GET", "/triggers/:id/text", triggerController.GetHumanText)
	router.Handle("GET", "/triggers/:id", triggerController.Get)
	router.Handle("POST", "/triggers", triggerController.Add)
	router.Handle("PUT", "/triggers/:id", triggerController.Update)
	router.Handle("DELETE", "/triggers/:id", triggerController.Delete)
	// manage pipelines attached to trigger
	router.Handle("GET", "/triggers/:id/pipelines", triggerController.GetPipelines)
	router.Handle("POST", "/triggers/:id/pipelines", triggerController.AddPipelines)
	router.Handle("DELETE", "/triggers/:id/pipelines/:pid", triggerController.DeletePipeline)
	// invoke trigger with event payload
	router.Handle("POST", "/trigger/:id", triggerController.TriggerEvent)
	// status handlers
	router.GET("/health", triggerController.GetHealth)
	router.GET("/version", triggerController.GetVersion)
	router.GET("/ping", triggerController.Ping)

	return router.Run(fmt.Sprintf(":%d", c.Int("port")))
}

// get triggers by name(s), filter or ALL
func getTriggers(c *cli.Context) error {
	quiet := c.Bool("quiet")
	triggerService := backend.NewRedisStore(c.GlobalString("redis"), c.GlobalInt("redis-port"), c.GlobalString("redis-password"), nil)
	if len(c.Args()) == 0 {
		triggers, err := triggerService.List(c.String("filter"))
		if err != nil {
			log.Error(err)
			return err
		}
		if len(triggers) == 0 {
			fmt.Println("No triggers defined!")
		}
		for _, t := range triggers {
			if quiet {
				fmt.Println(t.Event)
			} else {
				fmt.Println(t)
			}
		}
	} else {
		for _, id := range c.Args() {
			trigger, err := triggerService.Get(id)
			if err != nil {
				log.Error(err)
				return err
			}
			if quiet {
				fmt.Println(trigger.Event)
			} else {
				fmt.Println(trigger)
			}
		}
	}

	return nil
}

func getTriggerPipelines(c *cli.Context) error {
	triggerService := backend.NewRedisStore(c.GlobalString("redis"), c.GlobalInt("redis-port"), c.GlobalString("redis-password"), nil)
	if len(c.Args()) != 1 {
		return errors.New("wrong arguments: expected event URI")
	}
	pipelines, err := triggerService.GetPipelines(c.Args().First())
	if err != nil {
		log.Error(err)
		return err
	}
	for _, p := range pipelines {
		fmt.Println(p)
	}

	return nil
}

func addTriggerPipelines(c *cli.Context) error {
	// get trigger name and pipeline
	args := c.Args()
	if len(args) != 4 {
		return errors.New("wrong number of arguments")
	}
	// get codefresh endpoint
	codefreshService := codefresh.NewCodefreshEndpoint(c.GlobalString("cf"), c.GlobalString("t"))
	// get trigger service
	triggerService := backend.NewRedisStore(c.GlobalString("redis"), c.GlobalInt("redis-port"), c.GlobalString("redis-password"), codefreshService)
	// create pipelines
	pipelines := make([]model.Pipeline, 1)
	pipelines[0] = model.Pipeline{RepoOwner: args.Get(1), RepoName: args.Get(2), Name: args.Get(3)}
	return triggerService.AddPipelines(args.First(), pipelines)
}

func deleteTriggerPipeline(c *cli.Context) error {
	// get trigger name and pipeline
	args := c.Args()
	if len(args) != 4 {
		return errors.New("wrong number of arguments")
	}
	// get trigger service
	triggerService := backend.NewRedisStore(c.GlobalString("redis"), c.GlobalInt("redis-port"), c.GlobalString("redis-password"), nil)
	// create pipelines
	pid := fmt.Sprintf("%s:%s:%s", args.Get(1), args.Get(2), args.Get(3))
	return triggerService.DeletePipeline(c.Args().First(), pid)
}

// add new trigger
func addTrigger(c *cli.Context) error {
	// get trigger name and pipeline
	args := c.Args()
	if len(args) != 4 {
		return errors.New("wrong arguments")
	}
	// get codefresh endpoint
	codefreshService := codefresh.NewCodefreshEndpoint(c.GlobalString("cf"), c.GlobalString("t"))
	// get trigger service
	triggerService := backend.NewRedisStore(c.GlobalString("redis"), c.GlobalInt("redis-port"), c.GlobalString("redis-password"), codefreshService)
	// create trigger model
	trigger := model.Trigger{}
	trigger.Event = args.First()
	trigger.Secret = c.String("secret")
	trigger.Pipelines = make([]model.Pipeline, 1)
	trigger.Pipelines[0] = model.Pipeline{RepoOwner: args.Get(1), RepoName: args.Get(2), Name: args.Get(3)}
	return triggerService.Add(trigger)
}

// add new trigger
func deleteTrigger(c *cli.Context) error {
	// get trigger name
	args := c.Args()
	if len(args) != 1 {
		return errors.New("wrong argument, expected trigger event URI")
	}
	// get trigger service
	triggerService := backend.NewRedisStore(c.GlobalString("redis"), c.GlobalInt("redis-port"), c.GlobalString("redis-password"), nil)
	return triggerService.Delete(args.First())
}

// run all pipelines connected to specified trigger
func testTrigger(c *cli.Context) error {
	// get codefresh endpoint
	codefreshService := codefresh.NewCodefreshEndpoint(c.GlobalString("cf"), c.GlobalString("t"))
	// get trigger service
	triggerService := backend.NewRedisStore(c.GlobalString("redis"), c.GlobalInt("redis-port"), c.GlobalString("redis-password"), codefreshService)
	// convert command line 'var' variables (key=value) to map
	vars := make(map[string]string)
	for _, v := range c.StringSlice("var") {
		kv := strings.Split(v, "=")
		if len(kv) != 2 {
			return fmt.Errorf("Invalid 'var' value: %s ; should be 'key=value' form", v)
		}
		vars[kv[0]] = kv[1]
	}

	// get trigger from argument
	runs, err := triggerService.Run(c.Args().First(), vars)
	if err != nil {
		return err
	}
	fmt.Printf("Running %d pipelines ...\n", len(runs))
	for _, r := range runs {
		fmt.Println("\t", r)
	}
	return nil
}
