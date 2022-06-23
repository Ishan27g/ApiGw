package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"

	"github.com/Ishan27g/ApiGw/pkg"
)

func catchExit() chan bool {
	stop := make(chan bool, 1)
	closeLogs := make(chan os.Signal, 1)
	signal.Notify(closeLogs, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-closeLogs
		stop <- true
	}()
	return stop
}

var startCmd = cli.Command{
	Name:            "start",
	Aliases:         []string{"a"},
	Usage:           "start api gateway",
	HideHelp:        false,
	HideHelpCommand: false,
	Action: func(c *cli.Context) error {
		var apiGw pkg.ApiGw
		if c.Args().Len() == 0 {
			apiGw = pkg.NewFromConfig(pkg.Config{
				Listen: ":9999",
				Check:  true,
				Upstreams: []*pkg.Upstream{
					{
						Name:      "service-1",
						Addr:      "http://localhost:5999",
						UrlPrefix: "/any",
					},
				},
			})
		} else {
			apiGw = pkg.NewFromFile(c.Args().First())
		}
		stop := catchExit()
		if apiGw == nil {
			return cli.Exit("gateway error", 1)
		}
		go apiGw.Start(stop)
		<-stop
		return nil
	},
}
var checkCmd = cli.Command{
	Name:            "check",
	Aliases:         []string{"c"},
	Usage:           "check a task to the list",
	ArgsUsage:       "pkg check {path to config.hcl}",
	HideHelp:        false,
	HideHelpCommand: false,
	Action: func(c *cli.Context) error {
		if c.Args().Len() == 0 {
			return cli.Exit("filename not provided", 1)
		}
		_, err := pkg.ReadConfFile(c.Args().First())
		fmt.Println(err.Error())
		return nil
	},
}

func main() {
	app := &cli.App{
		Commands: []*cli.Command{
			&startCmd, &checkCmd,
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
