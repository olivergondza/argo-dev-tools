package run

import (
	"fmt"
	"os"
)

type project interface {
	Name() string
	CheckRepo() error
	Commands() []ProjectCommand
}

type ProjectCommand interface {
	Name() string
	Run() error
}

var ProjectRegistry = make(map[string]project)

func main() {
	args := os.Args
	if len(args) < 3 {
		usage()
		os.Exit(0)
	}

	project, command, err := findCommand(args[1], args[2])
	if err != nil {
		Out(os.Stderr, "Unknown command for '%s' '%s'", args[1], args[2])
		usage()
		os.Exit(1)
	}

	err = project.CheckRepo()
	if err != nil {
		Out(os.Stderr, "Failed running %s %s: %s", project.Name(), command.Name(), err.Error())
		os.Exit(2)
	}

	// Run is expected to run until ^C, so normal completion is a symptom of a problem
	err = command.Run()
	if err != nil {
		Out(os.Stderr, "Failed running %s %s: %s", project.Name(), command.Name(), err.Error())
		os.Exit(3)
	}

	Out(os.Stderr, "Command completed normally")
	Out(os.Stdout, "Command completed normally")
}

func usage() {
	Out(os.Stderr, "Usage: dev-tools/run PROJECT COMMAND")
	for _, project := range ProjectRegistry {
		for _, command := range project.Commands() {
			Out(os.Stderr, "    dev-tools/run %s %s", project.Name(), command.Name())
		}
	}
}

func findCommand(projectName string, commandName string) (project, ProjectCommand, error) {
	if project, ok := ProjectRegistry[projectName]; ok {
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

func Out(file *os.File, msg string, fmtArgs ...any) {
	if _, err := fmt.Fprintf(file, msg+"\n", fmtArgs...); err != nil {
		panic(err)
	}
}
