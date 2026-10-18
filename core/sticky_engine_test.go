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

func confirmStickyFailure(t *testing.T, fake *stickyFake) stickyFailureState {
	t.Helper()
	var failures stickyFailureState
	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyFailurePending)
	assertProbed(t, fake, []string{fake.snapshot.Current})
	assertCommitted(t, fake, nil)
	fake.probed = nil
	return failures
}

func TestStickyStepKeepsHealthyCurrentWithoutTouchingBackups(t *testing.T) {
	fake := newStickyFake("a", "a", "b", "c")
	fake.healthy["a"] = true
	fake.healthy["b"] = true
	fake.healthy["c"] = true

	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &stickyFailureState{}), stickyCurrentHealthy)
	assertProbed(t, fake, []string{"a"})
	assertCommitted(t, fake, nil)
}

func TestStickyStepDoesNotReclaimRecoveredCurrent(t *testing.T) {
	fake := newStickyFake("a", "a", "b")
	fake.healthy["b"] = true

	failures := confirmStickyFailure(t, fake)
	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyCommitted)
	assertCommitted(t, fake, []string{"b"})

	fake.probed = nil
	fake.healthy["a"] = true

	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyCurrentHealthy)
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

	failures := confirmStickyFailure(t, fake)
	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyCommitted)
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

	failures := confirmStickyFailure(t, fake)
	assertOutcome(t, stickyStep(ctx, fake, fake.snapshot, &failures), stickyCommitted)
	assertProbed(t, fake, members)
	assertCommitted(t, fake, []string{"j"})
}

func TestStickyStepRetainsCurrentWhenNoMemberIsHealthy(t *testing.T) {
	fake := newStickyFake("a", "a", "b", "c")

	failures := confirmStickyFailure(t, fake)
	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyNoCandidate)
	assertProbed(t, fake, []string{"a", "b", "c"})
	assertCommitted(t, fake, nil)
}

func TestStickyStepDoesNotSwitchOnCurrentProbeError(t *testing.T) {
	fake := newStickyFake("a", "a", "b")
	fake.healthy["b"] = true
	fake.probeErrs["a"] = errors.New("probe budget exhausted")

	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &stickyFailureState{}), stickyProbeError)
	assertProbed(t, fake, []string{"a"})
	assertCommitted(t, fake, nil)
}

func TestStickyStepUsesHealthyCandidateDespiteAnotherProbeError(t *testing.T) {
	fake := newStickyFake("a", "a", "b", "c")
	fake.healthy["c"] = true
	fake.probeErrs["b"] = errors.New("probe budget exhausted")

	failures := confirmStickyFailure(t, fake)
	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyCommitted)
	assertProbed(t, fake, []string{"a", "b", "c"})
	assertCommitted(t, fake, []string{"c"})
}

func TestStickyStepStopsBeforeProbingWhenCanceled(t *testing.T) {
	fake := newStickyFake("a", "a", "b")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assertOutcome(t, stickyStep(ctx, fake, fake.snapshot, &stickyFailureState{}), stickyCanceled)
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

	failures := confirmStickyFailure(t, fake)
	assertOutcome(t, stickyStep(ctx, fake, fake.snapshot, &failures), stickyCanceled)
	assertProbed(t, fake, []string{"a", "b", "c"})
	assertCommitted(t, fake, nil)
}

func TestStickyStepEndsRoundOnRejectedCommit(t *testing.T) {
	fake := newStickyFake("a", "a", "b", "c", "d")
	fake.healthy["b"] = true
	fake.accept = false

	failures := confirmStickyFailure(t, fake)
	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyCommitRejected)
	assertProbed(t, fake, []string{"a", "b", "c", "d"})
	assertCommitted(t, fake, []string{"b"})
}

func TestStickyStepResetsFailuresAfterRecovery(t *testing.T) {
	fake := newStickyFake("a", "a", "b")
	fake.healthy["b"] = true
	failures := confirmStickyFailure(t, fake)
	fake.healthy["a"] = true
	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyCurrentHealthy)
	if failures.failures != 0 {
		t.Fatalf("failures = %d after recovery, want 0", failures.failures)
	}
	fake.healthy["a"] = false
	fake.probed = nil
	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyFailurePending)
	assertProbed(t, fake, []string{"a"})
	assertCommitted(t, fake, nil)
	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyCommitted)
	assertCommitted(t, fake, []string{"b"})
}

func TestStickyStepProbeErrorDoesNotConfirmFailure(t *testing.T) {
	for _, probeErr := range []error{context.DeadlineExceeded, context.Canceled, errors.New("stale snapshot")} {
		t.Run(probeErr.Error(), func(t *testing.T) {
			fake := newStickyFake("a", "a", "b")
			fake.healthy["b"] = true
			failures := confirmStickyFailure(t, fake)
			fake.probeErrs["a"] = probeErr
			assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyProbeError)
			if failures.failures != 0 {
				t.Fatalf("failures = %d after probe error, want 0", failures.failures)
			}
			delete(fake.probeErrs, "a")
			fake.probed = nil
			assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyFailurePending)
			assertProbed(t, fake, []string{"a"})
			assertCommitted(t, fake, nil)
			assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyCommitted)
		})
	}
}

func TestStickyStepResetsFailuresForNewSelectionSnapshot(t *testing.T) {
	for _, change := range []string{"current", "revision", "group"} {
		t.Run(change, func(t *testing.T) {
			fake := newStickyFake("a", "a", "b", "c")
			fake.healthy["b"] = true
			failures := confirmStickyFailure(t, fake)
			switch change {
			case "current":
				fake.snapshot.Current = "c"
			case "revision":
				fake.snapshot.Revision++
			case "group":
				fake.snapshot.Group = "other"
			}
			assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyFailurePending)
			assertProbed(t, fake, []string{fake.snapshot.Current})
			assertCommitted(t, fake, nil)
			assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyCommitted)
			assertCommitted(t, fake, []string{"b"})
			if failures.failures != 0 {
				t.Fatalf("failures = %d after switch, want 0", failures.failures)
			}
		})
	}
}

func TestStickyStepCancellationClearsFailureConfirmation(t *testing.T) {
	fake := newStickyFake("a", "a", "b")
	fake.healthy["b"] = true
	failures := confirmStickyFailure(t, fake)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake.onProbe = func(context.Context, string) { cancel() }
	assertOutcome(t, stickyStep(ctx, fake, fake.snapshot, &failures), stickyCanceled)
	assertProbed(t, fake, []string{"a"})
	assertCommitted(t, fake, nil)
	if failures.failures != 0 {
		t.Fatalf("failures = %d after cancellation, want 0", failures.failures)
	}
}

func TestStickyStepKeepsConfirmedFailureWhenNoBackupWorks(t *testing.T) {
	fake := newStickyFake("a", "a", "b")
	failures := confirmStickyFailure(t, fake)
	for i := 0; i < 3; i++ {
		fake.probed = nil
		assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyNoCandidate)
		assertProbed(t, fake, []string{"a", "b"})
		if failures.failures != stickyFailureThreshold {
			t.Fatalf("failures = %d, want capped threshold", failures.failures)
		}
	}
	assertCommitted(t, fake, nil)
	fake.healthy["a"] = true
	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyCurrentHealthy)
	if failures.failures != 0 {
		t.Fatalf("failures = %d after recovery, want 0", failures.failures)
	}
}

func TestRunStickyGroupConfirmsFailureAcrossRounds(t *testing.T) {
	fake := newStickyFake("a", "a", "b")
	fake.healthy["b"] = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake.onSnapshot = func() {
		if len(fake.committed) != 0 {
			cancel()
		}
	}
	runStickyGroup(ctx, fake, "auto", 0, 0)
	assertProbed(t, fake, []string{"a", "a", "b"})
	assertCommitted(t, fake, []string{"b"})
}

func TestRunStickyGroupUnavailableSnapshotResetsFailureConfirmation(t *testing.T) {
	fake := newStickyFake("a", "a", "b")
	fake.healthy["b"] = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	fake.onSnapshot = func() {
		calls++
		fake.available = calls != 2
		if calls == 4 {
			fake.healthy["a"] = true
		}
		if calls == 5 {
			cancel()
		}
	}
	runStickyGroup(ctx, fake, "auto", 0, 0)
	assertProbed(t, fake, []string{"a", "a", "a"})
	assertCommitted(t, fake, nil)
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

	for _, outcome := range []stickyOutcome{stickyCurrentHealthy, stickyFailurePending, stickyCommitted, stickyCommitRejected, stickyProbeError} {
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
