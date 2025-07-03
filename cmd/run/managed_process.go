package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func osExec(args ...string) error {
	cp := newManagedProc(args...)
	cp.cmd.Stderr = os.Stderr
	cp.cmd.Stdout = os.Stdout
	if err := cp.run(); err != nil {
		return err
	}

	return nil
}

type managedProc struct {
	visual string
	cmd    *exec.Cmd

	releaseContextTask func()
	status             managedProcStatus
}

type managedProcStatus = string

func newManagedProc(args ...string) *managedProc {
	mp := &managedProc{
		visual: fmt.Sprintf("$ %s", strings.Join(args, " ")),
	}
	mp.update("new") // Set status this way so the transition is logged

	command := args[0]
	args = args[1:]

	var ctx context.Context
	ctx, mp.releaseContextTask = mainTt.useContext()
	mp.cmd = exec.CommandContext(ctx, command, args...)
	return mp
}

func (mp *managedProc) run() error {
	mp.update("running")
	// Keep waiting for as long as cmd.Run() is running
	defer func() {
		_ = os.Stderr.Sync()
		_ = os.Stdout.Sync()
		out(os.Stderr, "outs synced")

		mp.releaseContextTask()

	}()
	err := mp.cmd.Run()
	if err != nil {
		mp.update(fmt.Sprintf("failed(%s)", err.Error()))
		return fmt.Errorf("failed: %s", err)
	}

	mp.update("completed")

	return nil
}

func (mp *managedProc) String() string {
	return fmt.Sprintf("%v: %s", mp.status, mp.visual)
}

func (mp *managedProc) update(status managedProcStatus) {
	mp.status = status
	out(os.Stderr, "managedProcess: %v", mp)
}
