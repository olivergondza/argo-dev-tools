package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

func osExec(args ...string) error {
	cp := newManagedProc(args...)
	registerCleanup(cp.interrupt)
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
	// Context used to cancel running command
	cancel func()

	running *sync.WaitGroup
	status  managedProcStatus
}

type managedProcStatus = string

func newManagedProc(args ...string) *managedProc {
	running := &sync.WaitGroup{}
	running.Add(1)

	mp := &managedProc{
		visual:  fmt.Sprintf("$ %s", strings.Join(args, " ")),
		running: running,
	}
	mp.update("new") // Set status this way so the transition is logged

	command := args[0]
	args = args[1:]

	var ctx context.Context
	ctx, mp.cancel = context.WithCancel(context.Background())
	mp.cmd = exec.CommandContext(ctx, command, args...)
	return mp
}

func (mp *managedProc) run() error {
	mp.update("running")
	// Keep waiting for as long as cmd.Run() is running
	defer func() {
		mp.running.Done()

		_ = os.Stderr.Sync()
		_ = os.Stdout.Sync()
		out(os.Stderr, "outs synced")
	}()
	err := mp.cmd.Run()
	if err != nil {
		mp.update("failed")
		return fmt.Errorf("failed: %s", err)
	}

	mp.update("completed")

	return nil
}

func (mp *managedProc) interrupt() {
	if mp.status == "completed" {
		return // noop
	}

	mp.update(fmt.Sprintf("interrupting... (was %s)", mp.status))
	mp.running.Wait()
	mp.update("interrupted")
}

func (mp *managedProc) String() string {
	return fmt.Sprintf("%v: %s", mp.status, mp.visual)
}

func (mp *managedProc) update(status managedProcStatus) {
	mp.status = status
	out(os.Stderr, "managedProcess: %v", mp)
}
