package main

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var cleanStack []func()
var cleanLock sync.Mutex

func registerCleanup(cleaner func()) {
	cleanLock.Lock()
	defer cleanLock.Unlock()
	cleanStack = append(cleanStack, cleaner)
}

func cleanAll() {
	cleanLock.Lock()
	defer cleanLock.Unlock()

	stackLen := len(cleanStack)
	out(os.Stderr, "Registered cleaners: %d", stackLen)

	for len(cleanStack) > 0 {
		last := len(cleanStack) - 1
		cleaner := cleanStack[last]
		cleanStack = cleanStack[:last]
		out(os.Stderr, "Cleaning %v (%d)", cleaner, last)
		cleaner()
	}
}

func init() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go onSignal(signals)
}

func onSignal(signals chan os.Signal) {
	sig := <-signals
	out(os.Stderr, "Caught signal %v", sig)

	cleanAll()

	_ = os.Stderr.Sync()
	_ = os.Stdout.Sync()

	os.Exit(42)
}
