package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var mainTt *taskTracker

type taskTracker struct {
	ctx    context.Context
	cancel func()
	count  *sync.WaitGroup
}

func (t *taskTracker) useContext(name string) (context.Context, func()) {
	t.count.Add(1)
	out(os.Stderr, "CONTEXT USED "+name)
	return t.ctx, func() {
		out(os.Stderr, "CONTEXT RETURNED "+name)
		t.count.Done()
	}
}

func wasInterrupted() bool {
	select {
	case <-mainTt.ctx.Done():
		return true
	default:
		return false
	}
}

func init() {
	ctx, cancel := context.WithCancel(context.Background())
	mainTt = &taskTracker{ctx, cancel, &sync.WaitGroup{}}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go onSignal(signals)
}

func onSignal(signals chan os.Signal) {
	sig := <-signals
	out(os.Stderr, "Caught signal %v", sig)

	mainTt.cancel()
	out(os.Stderr, "Waiting for tasks to complete")
	mainTt.count.Wait()
	out(os.Stderr, "All tasks completed")

	os.Exit(42)
}
