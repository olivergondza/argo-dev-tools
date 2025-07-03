package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

func osExec(args ...string) error {
	mp := newManagedProc(args...)
	if err := mp.run(); err != nil {
		return err
	}

	return nil
}

type managedProc struct {
	visual            string
	cmd               *exec.Cmd
	stdoutTransformer func(in string) string

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
	ctx, mp.releaseContextTask = mainTt.useContext("process-" + mp.visual)
	mp.cmd = exec.CommandContext(ctx, command, args...)

	// Start all children processes in one process group to deliver the SIGTERM in one go.
	// This speeds up termination of goreman significantly.
	mp.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Replace default handler from exec.CommandContext, use SIGTERM over SIGKILL.
	mp.cmd.Cancel = func() error {
		err := mp.cmd.Process.Signal(syscall.SIGTERM)
		if err != nil {
			return err
		}
		err = mp.cmd.Process.Signal(syscall.SIGTERM)
		if err != nil {
			return err
		}
		return nil
	}
	return mp
}

func (mp *managedProc) run() error {
	outputsWritten, err := mp.pumpOutputs()
	if err != nil {
		return err
	}

	mp.update("running")
	// Keep waiting for as long as cmd.Run() is running
	defer func() {
		mp.releaseContextTask()
	}()

	err = mp.cmd.Run()
	if err != nil {
		mp.update(fmt.Sprintf("failed(%s)", err.Error()))
		return fmt.Errorf("failed: %s", err)
	}

	mp.update("flushing-outs")

	outputsWritten.Wait()

	mp.update("completed")

	return nil
}

func (mp *managedProc) pumpOutputs() (*sync.WaitGroup, error) {
	var wg sync.WaitGroup
	wg.Add(2)
	outPipe, err := mp.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	errPipe, err := mp.cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	outPump := &streamPump{outPipe, os.Stdout, mp.stdoutTransformer, &wg}
	errPump := &streamPump{errPipe, os.Stderr, nil, &wg}
	go outPump.pump()
	go errPump.pump()
	return &wg, err
}

func (mp *managedProc) String() string {
	return fmt.Sprintf("%v: %s", mp.status, mp.visual)
}

func (mp *managedProc) update(status managedProcStatus) {
	mp.status = status
	out(os.Stderr, "managedProcess: %v", mp)
}

type streamPump struct {
	reader      io.ReadCloser
	writer      io.Writer
	transformer func(string) string
	done        *sync.WaitGroup
}

func (sp *streamPump) pump() {
	defer sp.done.Done() // Report when all output is processed

	// Noop if nil transformer configured
	if sp.transformer == nil {
		sp.transformer = func(s string) string { return s }
	}

	scanner := bufio.NewScanner(sp.reader)
	for scanner.Scan() {
		line := sp.transformer(scanner.Text())

		_, err := fmt.Fprintf(sp.writer, "%s\n", line)
		if err != nil {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		out(os.Stderr, "Error reading from pipe: %v", err)
	}
}
