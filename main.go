package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"

	"github.com/Ishan27g/ApiGw/gw"
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
		if c.Args().Len() == 0 {
			return cli.Exit("filename not provided", 1)
		}
		stop := catchExit()
		apiGw := gw.NewFromFile(c.Args().First())
		if apiGw == nil {
			return cli.Exit("gateway error", 1)
		}
		apiGw.Start(stop)
		<-stop
		return nil
	},
}
var checkCmd = cli.Command{
	Name:            "check",
	Aliases:         []string{"c"},
	Usage:           "check a task to the list",
	ArgsUsage:       "apiGw check {path to config.hcl}",
	HideHelp:        false,
	HideHelpCommand: false,
	Action: func(c *cli.Context) error {
		if c.Args().Len() == 0 {
			return cli.Exit("filename not provided", 1)
		}
		_, err := gw.ReadConfFile(c.Args().First())
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
