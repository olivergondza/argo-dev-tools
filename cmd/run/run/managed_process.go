package run

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

// lineTransformer optionally mutates lines of a ManagedProc output.
// Modified string will be printed, line is omitted is nil is returned.
type lineTransformer func(in string) *string

type ManagedProc struct {
	visual            string
	cmd               *exec.Cmd
	StdoutTransformer lineTransformer
	StderrTransformer lineTransformer

	releaseContextTask func()
	status             managedProcStatus
}

type managedProcStatus = string

func NewManagedProc(args ...string) *ManagedProc {
	mp := &ManagedProc{
		visual: fmt.Sprintf("$ %s", strings.Join(args, " ")),
	}
	mp.update("new") // Set status this way so the transition is logged

	command := args[0]
	args = args[1:]

	var ctx context.Context
	ctx, mp.releaseContextTask = MainTt.UseContext("process-" + mp.visual)
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

func (mp *ManagedProc) CaptureStdout() *bytes.Buffer {
	buffer := new(bytes.Buffer)
	mp.StdoutTransformer = func(in string) *string {
		buffer.WriteString(in)
		buffer.WriteString("\n")
		return nil
	}
	return buffer
}

func (mp *ManagedProc) Dir(cwd string) {
	mp.cmd.Dir = cwd
}

func (mp *ManagedProc) AddEnv(key string, value string) {
	if mp.cmd.Env == nil {
		mp.cmd.Env = os.Environ()
	}
	mp.cmd.Env = append(mp.cmd.Env, key+"="+value)
}

func (mp *ManagedProc) Run() error {
	Out(os.Stderr, mp.visual)

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

func (mp *ManagedProc) pumpOutputs() (*sync.WaitGroup, error) {
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
	outPump := &streamPump{outPipe, os.Stdout, mp.StdoutTransformer, &wg}
	errPump := &streamPump{errPipe, os.Stderr, mp.StderrTransformer, &wg}
	go outPump.pump()
	go errPump.pump()
	return &wg, err
}

func (mp *ManagedProc) String() string {
	return fmt.Sprintf("%v: %s", mp.status, mp.visual)
}

func (mp *ManagedProc) update(status managedProcStatus) {
	mp.status = status
}

type streamPump struct {
	reader      io.ReadCloser
	writer      io.Writer
	transformer lineTransformer
	done        *sync.WaitGroup
}

func (sp *streamPump) pump() {
	defer sp.done.Done() // Report when all output is processed

	// Noop if nil transformer configured
	if sp.transformer == nil {
		sp.transformer = func(s string) *string { return &s }
	}

	scanner := bufio.NewScanner(sp.reader)
	for scanner.Scan() {
		line := sp.transformer(scanner.Text())

		if line != nil {
			_, err := fmt.Fprintf(sp.writer, "%s\n", *line)
			if err != nil {
				return
			}
		}
	}
	if err := scanner.Err(); err != nil {
		Out(os.Stderr, "Error reading from pipe: %v", err)
	}
}
