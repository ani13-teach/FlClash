package main

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"
)

type stickyFake struct {
	mu         sync.Mutex
	snapshot   stickySnapshot
	available  bool
	healthy    map[string]bool
	probeErrs  map[string]error
	accept     bool
	onProbe    func(context.Context, string)
	onSnapshot func()
	probed     []string
	committed  []string
}

func newStickyFake(current string, members ...string) *stickyFake {
	return &stickyFake{
		snapshot:  stickySnapshot{Group: "auto", Current: current, Members: members, Revision: 1},
		available: true,
		healthy:   map[string]bool{},
		probeErrs: map[string]error{},
		accept:    true,
	}
}

func (fake *stickyFake) Snapshot(string) (stickySnapshot, bool) {
	if fake.onSnapshot != nil {
		fake.onSnapshot()
	}
	if !fake.available {
		return stickySnapshot{}, false
	}
	return fake.snapshot, true
}

func (fake *stickyFake) Probe(ctx context.Context, _ stickySnapshot, name string) (bool, error) {
	fake.mu.Lock()
	fake.probed = append(fake.probed, name)
	fake.mu.Unlock()
	if fake.onProbe != nil {
		fake.onProbe(ctx, name)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := fake.probeErrs[name]; err != nil {
		return false, err
	}
	return fake.healthy[name], nil
}

func (fake *stickyFake) Commit(_ stickySnapshot, candidate string) bool {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.committed = append(fake.committed, candidate)
	if !fake.accept {
		return false
	}
	fake.snapshot.Current = candidate
	fake.snapshot.Revision++
	return true
}

func assertOutcome(t *testing.T, got, want stickyOutcome) {
	t.Helper()
	if got != want {
		t.Fatalf("outcome = %d, want %d", got, want)
	}
}

func assertProbed(t *testing.T, fake *stickyFake, want []string) {
	t.Helper()
	fake.mu.Lock()
	got := slices.Clone(fake.probed)
	fake.mu.Unlock()
	if len(got) > 1 {
		slices.Sort(got[1:])
	}
	if len(want) > 1 {
		slices.Sort(want[1:])
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("probed = %v, want %v", got, want)
	}
}

func assertCommitted(t *testing.T, fake *stickyFake, want []string) {
	t.Helper()
	if !reflect.DeepEqual(fake.committed, want) {
		t.Errorf("committed = %v, want %v", fake.committed, want)
	}
}

func TestStickyStepKeepsHealthyCurrentWithoutTouchingBackups(t *testing.T) {
	fake := newStickyFake("a", "a", "b", "c")
	fake.healthy["a"] = true
	fake.healthy["b"] = true
	fake.healthy["c"] = true

	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot), stickyCurrentHealthy)
	assertProbed(t, fake, []string{"a"})
	assertCommitted(t, fake, nil)
}

func TestStickyStepDoesNotReclaimRecoveredCurrent(t *testing.T) {
	fake := newStickyFake("a", "a", "b")
	fake.healthy["b"] = true

	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot), stickyCommitted)
	assertCommitted(t, fake, []string{"b"})

	fake.probed = nil
	fake.healthy["a"] = true

	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot), stickyCurrentHealthy)
	assertProbed(t, fake, []string{"b"})
	assertCommitted(t, fake, []string{"b"})
}

func TestStickyStepSwitchesToFirstHealthyMember(t *testing.T) {
	fake := newStickyFake("a", "a", "b", "c", "d")
	fake.healthy["c"] = true
	fake.healthy["d"] = true
	fake.onProbe = func(ctx context.Context, name string) {
		if name == "d" {
			<-ctx.Done()
		}
	}

	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot), stickyCommitted)
	assertProbed(t, fake, []string{"a", "b", "c", "d"})
	assertCommitted(t, fake, []string{"c"})
}

func TestStickyStepDoesNotWaitForEarlierUnhealthyCandidates(t *testing.T) {
	members := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	fake := newStickyFake("a", members...)
	fake.healthy["j"] = true
	fake.onProbe = func(ctx context.Context, name string) {
		if name != "a" && name != "j" {
			<-ctx.Done()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	assertOutcome(t, stickyStep(ctx, fake, fake.snapshot), stickyCommitted)
	assertProbed(t, fake, members)
	assertCommitted(t, fake, []string{"j"})
}

func TestStickyStepRetainsCurrentWhenNoMemberIsHealthy(t *testing.T) {
	fake := newStickyFake("a", "a", "b", "c")

	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot), stickyNoCandidate)
	assertProbed(t, fake, []string{"a", "b", "c"})
	assertCommitted(t, fake, nil)
}

func TestStickyStepDoesNotSwitchOnCurrentProbeError(t *testing.T) {
	fake := newStickyFake("a", "a", "b")
	fake.healthy["b"] = true
	fake.probeErrs["a"] = errors.New("probe budget exhausted")

	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot), stickyProbeError)
	assertProbed(t, fake, []string{"a"})
	assertCommitted(t, fake, nil)
}

func TestStickyStepUsesHealthyCandidateDespiteAnotherProbeError(t *testing.T) {
	fake := newStickyFake("a", "a", "b", "c")
	fake.healthy["c"] = true
	fake.probeErrs["b"] = errors.New("probe budget exhausted")

	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot), stickyCommitted)
	assertProbed(t, fake, []string{"a", "b", "c"})
	assertCommitted(t, fake, []string{"c"})
}

func TestStickyStepStopsBeforeProbingWhenCanceled(t *testing.T) {
	fake := newStickyFake("a", "a", "b")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assertOutcome(t, stickyStep(ctx, fake, fake.snapshot), stickyCanceled)
	assertProbed(t, fake, nil)
}

func TestStickyStepStopsScanWhenCanceled(t *testing.T) {
	fake := newStickyFake("a", "a", "b", "c")
	fake.healthy["c"] = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake.onProbe = func(ctx context.Context, name string) {
		if name == "b" {
			cancel()
		} else if name == "c" {
			<-ctx.Done()
		}
	}

	assertOutcome(t, stickyStep(ctx, fake, fake.snapshot), stickyCanceled)
	assertProbed(t, fake, []string{"a", "b", "c"})
	assertCommitted(t, fake, nil)
}

func TestStickyStepEndsRoundOnRejectedCommit(t *testing.T) {
	fake := newStickyFake("a", "a", "b", "c", "d")
	fake.healthy["b"] = true
	fake.accept = false

	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot), stickyCommitRejected)
	assertProbed(t, fake, []string{"a", "b", "c", "d"})
	assertCommitted(t, fake, []string{"b"})
}

func TestRunStickyGroupWaitsForAvailableSnapshot(t *testing.T) {
	fake := newStickyFake("a", "a", "b")
	fake.available = false
	fake.healthy["a"] = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	fake.onSnapshot = func() {
		calls++
		fake.available = calls > 1
	}
	fake.onProbe = func(_ context.Context, _ string) { cancel() }

	runStickyGroup(ctx, fake, "auto", 0, 0)

	if calls != 2 {
		t.Fatalf("snapshot calls = %d, want 2", calls)
	}
	assertProbed(t, fake, []string{"a"})
	assertCommitted(t, fake, nil)
}

func TestRunStickyGroupReturnsOnCanceledContext(t *testing.T) {
	fake := newStickyFake("a", "a", "b")
	fake.healthy["a"] = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runStickyGroup(ctx, fake, "auto", time.Second, time.Second)

	assertProbed(t, fake, nil)
}

func TestRunStickyGroupStopsAfterACanceledRound(t *testing.T) {
	fake := newStickyFake("a", "a", "b")
	fake.healthy["a"] = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake.onProbe = func(_ context.Context, _ string) {
		cancel()
	}

	runStickyGroup(ctx, fake, "auto", time.Second, time.Second)

	assertProbed(t, fake, []string{"a"})
}

func TestStickyNextWaitHoldsCadenceForCompletedRounds(t *testing.T) {
	roundStart := time.Unix(1700000000, 0)
	now := roundStart.Add(3 * time.Second)

	for _, outcome := range []stickyOutcome{stickyCurrentHealthy, stickyCommitted, stickyCommitRejected, stickyProbeError} {
		wait := stickyNextWait(roundStart, now, outcome, 10*time.Second, 2*time.Second)
		if wait != 7*time.Second {
			t.Errorf("wait = %v for outcome %d, want 7s measured from the round start", wait, outcome)
		}
	}
}

func TestStickyNextWaitDoesNotWaitWhenRoundOverrunsInterval(t *testing.T) {
	roundStart := time.Unix(1700000000, 0)
	now := roundStart.Add(12 * time.Second)

	if wait := stickyNextWait(roundStart, now, stickyCommitted, 10*time.Second, 2*time.Second); wait != 0 {
		t.Errorf("wait = %v, want 0 for a round that overran the interval", wait)
	}
}

func TestStickyNextWaitBacksOffAfterFailedRound(t *testing.T) {
	roundStart := time.Unix(1700000000, 0)
	now := roundStart.Add(9 * time.Second)

	for _, outcome := range []stickyOutcome{stickyNoCandidate} {
		wait := stickyNextWait(roundStart, now, outcome, 10*time.Second, 2*time.Second)
		if wait != 2*time.Second {
			t.Errorf("wait = %v for outcome %d, want the full retry delay", wait, outcome)
		}
	}
}
