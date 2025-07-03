package main

import (
	"fmt"
	"os"
)

type project interface {
	Name() string
	Commands() []projectCommand
}

type projectCommand interface {
	Name() string
	Run() error
}

var projectRegistry = make(map[string]project)

func main() {
	args := os.Args
	if len(args) < 3 {
		usage()
		os.Exit(0)
	}

	project, command, err := findCommand(args[1], args[2])
	if err != nil {
		out(os.Stderr, "Unknown command for '%s' '%s'", args[1], args[2])
		usage()
		os.Exit(1)
	}

	// Run is expected to run until ^C, so normal completion is a symptom of a problem
	err = command.Run()
	if err != nil {
		out(os.Stderr, "Failed running %s %s: %s", project.Name(), command.Name(), err.Error())
		os.Exit(2)
	}

	out(os.Stderr, "Command completed normally")
	out(os.Stdout, "Command completed normally")
}

func usage() {
	out(os.Stderr, "Usage: dev-tools/run PROJECT COMMAND")
	for _, project := range projectRegistry {
		for _, command := range project.Commands() {
			out(os.Stderr, "    dev-tools/run %s %s", project.Name(), command.Name())
		}
	}
}

func findCommand(projectName string, commandName string) (project, projectCommand, error) {
	if project, ok := projectRegistry[projectName]; ok {
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

func out(file *os.File, msg string, fmtArgs ...any) {
	if _, err := fmt.Fprintf(file, msg+"\n", fmtArgs...); err != nil {
		panic(err)
	}
}
