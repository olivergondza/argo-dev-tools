package run

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var MainTt *taskTracker

type taskTracker struct {
	ctx    context.Context
	cancel func()
	count  *sync.WaitGroup
}

func (t *taskTracker) UseContext(name string) (context.Context, func()) {
	t.count.Add(1)
	Out(os.Stderr, "CONTEXT USED "+name)
	return t.ctx, func() {
		Out(os.Stderr, "CONTEXT RETURNED "+name)
		t.count.Done()
	}
}

func WasInterrupted() bool {
	select {
	case <-MainTt.ctx.Done():
		return true
	default:
		return false
	}
}

func init() {
	ctx, cancel := context.WithCancel(context.Background())
	MainTt = &taskTracker{ctx, cancel, &sync.WaitGroup{}}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go onSignal(signals)
}

func onSignal(signals chan os.Signal) {
	sig := <-signals
	Out(os.Stderr, "Caught signal %v", sig)

	MainTt.cancel()
	Out(os.Stderr, "Waiting for tasks to complete")
	MainTt.count.Wait()
	Out(os.Stderr, "All tasks completed")

	os.Exit(42)
}
