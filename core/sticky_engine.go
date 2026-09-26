package main

import (
	"context"
	"sync"
	"time"
)

type stickySnapshot struct {
	Group    string
	Current  string
	Members  []string
	Revision uint64
}

type stickyBackend interface {
	Snapshot(group string) (stickySnapshot, bool)
	Probe(ctx context.Context, snapshot stickySnapshot, name string) (bool, error)
	Commit(snapshot stickySnapshot, candidate string) bool
}

type stickyOutcome int

const (
	stickyCurrentHealthy stickyOutcome = iota
	stickyCommitted
	stickyCommitRejected
	stickyNoCandidate
	stickyProbeError
	stickyCanceled
)

func runStickyGroup(ctx context.Context, backend stickyBackend, group string, interval, retryDelay time.Duration) {
	// The loop sleeps between rounds rather than spawning, so rounds never overlap.
	for ctx.Err() == nil {
		roundStart := time.Now()
		snapshot, ok := backend.Snapshot(group)
		if !ok {
			if !sleepSticky(ctx, retryDelay) {
				return
			}
			continue
		}
		outcome := stickyStep(ctx, backend, snapshot)
		if ctx.Err() != nil {
			return
		}
		if !sleepSticky(ctx, stickyNextWait(roundStart, time.Now(), outcome, interval, retryDelay)) {
			return
		}
	}
}

func stickyStep(ctx context.Context, backend stickyBackend, snapshot stickySnapshot) stickyOutcome {
	if ctx.Err() != nil {
		return stickyCanceled
	}
	healthy, err := backend.Probe(ctx, snapshot, snapshot.Current)
	if err != nil {
		return stickyProbeError
	}
	if healthy {
		return stickyCurrentHealthy
	}
	probeCtx, cancel := context.WithCancel(ctx)
	type result struct {
		name    string
		healthy bool
		err     error
	}
	results := make(chan result, len(snapshot.Members))
	var probes sync.WaitGroup
	defer func() {
		cancel()
		probes.Wait()
	}()
	pending := 0
	for _, member := range snapshot.Members {
		if member == snapshot.Current {
			continue
		}
		pending++
		probes.Add(1)
		go func(name string) {
			defer probes.Done()
			healthy, err := backend.Probe(probeCtx, snapshot, name)
			results <- result{name: name, healthy: healthy, err: err}
		}(member)
	}
	hadError := false
	for pending > 0 {
		pending--
		select {
		case <-ctx.Done():
			return stickyCanceled
		case candidate := <-results:
			if candidate.err != nil {
				hadError = true
				continue
			}
			if !candidate.healthy {
				continue
			}
			if ctx.Err() != nil {
				return stickyCanceled
			}
			if backend.Commit(snapshot, candidate.name) {
				return stickyCommitted
			}
			return stickyCommitRejected
		}
	}
	if hadError {
		return stickyProbeError
	}
	return stickyNoCandidate
}

func stickyNextWait(roundStart, now time.Time, outcome stickyOutcome, interval, retryDelay time.Duration) time.Duration {
	switch outcome {
	case stickyNoCandidate:
		return retryDelay
	}
	remaining := roundStart.Add(interval).Sub(now)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func sleepSticky(ctx context.Context, wait time.Duration) bool {
	if wait <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
