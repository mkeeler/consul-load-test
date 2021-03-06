package main

import (
	"log"
	"os"

	"github.com/mitchellh/cli"
)

func bold(color cli.UiColor) cli.UiColor {
	newColor := color
	newColor.Bold = true
	return newColor
}

func main() {
	c := cli.NewCLI("consul-load", "0.0.1")
	c.Args = os.Args[1:]

	basic := &cli.BasicUi{
		Reader:      os.Stdin,
		Writer:      os.Stdout,
		ErrorWriter: os.Stderr,
	}

	ui := &cli.ColoredUi{
		OutputColor: cli.UiColorNone,
		InfoColor:   bold(cli.UiColorGreen),
		ErrorColor:  bold(cli.UiColorRed),
		WarnColor:   bold(cli.UiColorYellow),
		Ui:          basic,
	}

	c.Commands = map[string]cli.CommandFactory{
		"gen": func() (cli.Command, error) { return newLoadCommand(ui), nil },
	}

	exitStatus, err := c.Run()
	if err != nil {
		log.Println(err)
	}

	os.Exit(exitStatus)
}
