package main

import (
	"fmt"
	_ "github.com/argoproj/dev-tools/cmd/run/project" // For init(): Command registration
	"github.com/argoproj/dev-tools/cmd/run/run"
	"os"
)

func main() {
	args := os.Args
	if len(args) < 3 {
		usage()
		os.Exit(0)
	}

	project, command, err := findCommand(args[1], args[2])
	if err != nil {
		run.Out(os.Stderr, "Unknown command for '%s' '%s'", args[1], args[2])
		usage()
		os.Exit(1)
	}

	err = project.CheckRepo()
	if err != nil {
		run.Out(os.Stderr, "Failed running %s %s: %s", project.Name(), command.Name(), err.Error())
		os.Exit(2)
	}

	// Run is expected to run until ^C, so normal completion is a symptom of a problem
	err = command.Run()
	if err != nil {
		run.Out(os.Stderr, "Failed running %s %s: %s", project.Name(), command.Name(), err.Error())
		os.Exit(3)
	}

	run.Out(os.Stderr, "Command completed normally")
	run.Out(os.Stdout, "Command completed normally")
}

func usage() {
	run.Out(os.Stderr, "Usage: dev-tools/run PROJECT COMMAND")
	for _, project := range run.ProjectRegistry {
		for _, command := range project.Commands() {
			run.Out(os.Stderr, "    dev-tools/run %s %s", project.Name(), command.Name())
		}
	}
}

func findCommand(projectName string, commandName string) (run.Project, run.ProjectCommand, error) {
	if project, ok := run.ProjectRegistry[projectName]; ok {
		for _, command := range project.Commands() {
			if command.Name() == commandName {
				return project, command, nil
			}
		}
		return nil, nil, fmt.Errorf("unknown command `%s` for project: %s", commandName, projectName)
	} else {
		return nil, nil, fmt.Errorf("unknown project: %s", projectName)
	}
}
