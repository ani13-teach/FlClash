package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outboundgroup"
	"github.com/metacubex/mihomo/adapter/provider"
	"github.com/metacubex/mihomo/common/utils"
	"github.com/metacubex/mihomo/constant"
	cp "github.com/metacubex/mihomo/constant/provider"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/tunnel"
)

type stickyTestNode struct {
	constant.Proxy
	calls       atomic.Int32
	healthy     bool
	onProbe     func(context.Context)
	probeResult func(context.Context) (uint16, error)
}

func (p *stickyTestNode) Type() constant.AdapterType { return constant.Shadowsocks }

func (p *stickyTestNode) URLTest(ctx context.Context, _ string, _ utils.IntRanges[uint16]) (uint16, error) {
	p.calls.Add(1)
	if p.onProbe != nil {
		p.onProbe(ctx)
	}
	if p.probeResult != nil {
		return p.probeResult(ctx)
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if !p.healthy {
		return 0, errors.New("unreachable")
	}
	return 10, nil
}

func stickyNode(name string) *stickyTestNode {
	return &stickyTestNode{Proxy: namedProxy(name), healthy: true}
}

func stickyTestGroup(t *testing.T, name string, members ...constant.Proxy) constant.Proxy {
	t.Helper()
	health := provider.NewHealthCheck(members, "", 0, 0, true, nil)
	pd, err := provider.NewCompatibleProvider(name+"-provider", members, health)
	if err != nil {
		t.Fatal(err)
	}
	selector, err := outboundgroup.NewSelector(
		outboundgroup.GroupCommonOption{Name: name},
		outboundgroup.SelectorOption{}, nil, []cp.ProxyProvider{pd},
	)
	if err != nil {
		t.Fatal(err)
	}
	return adapter.NewProxy(selector)
}

func stickyTestBackend(t *testing.T, proxies map[string]constant.Proxy) *mihomoStickyBackend {
	t.Helper()
	previousHome := constant.Path.HomeDir()
	previousRunning, previousSuspended := isRunning.Load(), isSuspended.Load()
	previousScope, previousPath, previousSelections := stickyScope, stickyPath, stickySelections
	previousApplying, previousPending := stickyApplying, stickyPending
	stopSticky()
	constant.SetHomeDir(t.TempDir())
	isRunning.Store(true)
	isSuspended.Store(false)
	tunnel.UpdateProxies(proxies, nil)
	configureSticky([]byte("profile-a"), true)
	ctx, cancel := context.WithCancel(context.Background())
	backend := &mihomoStickyBackend{
		ctx: ctx, cancel: cancel,
		views:  make(map[string]stickyView),
		probes: make(map[constant.Proxy]*stickyProbe),
		slots:  make(chan struct{}, 16),
	}
	stickyRun = backend
	t.Cleanup(func() {
		stopSticky()
		tunnel.UpdateProxies(nil, nil)
		constant.SetHomeDir(previousHome)
		isRunning.Store(previousRunning)
		isSuspended.Store(previousSuspended)
		stickyScope, stickyPath, stickySelections = previousScope, previousPath, previousSelections
		stickyApplying, stickyPending = previousApplying, previousPending
	})
	return backend
}

func TestStickyBackendExcludesDirectAndNestedGroups(t *testing.T) {
	nested := selectorGroup(t, "nested", "inner")
	group := stickyTestGroup(t, "[Sticky] test", stickyNode("a"), namedProxy("DIRECT"), nested)
	backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
	snapshot, ok := backend.Snapshot(group.Name())
	if !ok || len(snapshot.Members) != 1 || snapshot.Members[0] != "a" {
		t.Fatalf("snapshot = %+v, available = %v", snapshot, ok)
	}
	if err := handleChangeProxy(&ChangeProxyParams{GroupName: group.Name(), ProxyName: "DIRECT"}); err != "" {
		t.Fatal(err)
	}
	if _, ok := backend.Snapshot(group.Name()); ok {
		t.Fatal("manual DIRECT selection must suspend automatic failover")
	}
}

func TestStickyBackendManualChoiceSupersedesScan(t *testing.T) {
	group := stickyTestGroup(t, "[Sticky] test", stickyNode("a"), stickyNode("b"), stickyNode("c"))
	backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
	snapshot, _ := backend.Snapshot(group.Name())
	if err := handleChangeProxy(&ChangeProxyParams{GroupName: group.Name(), ProxyName: "c"}); err != "" {
		t.Fatal(err)
	}
	if backend.Commit(snapshot, "b") || groupNow(t, group) != "c" {
		t.Fatal("obsolete scan overrode the manual choice")
	}
}

func TestStickyBackendRejectsReplacedGroupAndStoppedRun(t *testing.T) {
	group := stickyTestGroup(t, "[Sticky] test", stickyNode("a"), stickyNode("b"))
	backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
	snapshot, _ := backend.Snapshot(group.Name())
	replacement := stickyTestGroup(t, group.Name(), stickyNode("a"), stickyNode("b"))
	tunnel.UpdateProxies(map[string]constant.Proxy{group.Name(): replacement}, nil)
	if backend.Commit(snapshot, "b") {
		t.Fatal("obsolete snapshot selected a node on the new group")
	}
	snapshot, _ = backend.Snapshot(group.Name())
	stopSticky()
	if backend.Commit(snapshot, "b") {
		t.Fatal("stopped run committed a selection")
	}
	if _, err := os.Stat(stickyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled selections must not be persisted: %v", err)
	}
}

func TestStickyBackendProbesActualMemberAndCoalescesSharedNodes(t *testing.T) {
	actual := stickyNode("duplicate")
	other := stickyNode("duplicate")
	other.healthy = false
	first := stickyTestGroup(t, "[Sticky] first", actual)
	second := stickyTestGroup(t, "[Sticky] second", actual)
	third := stickyTestGroup(t, "[Sticky] third", other)
	backend := stickyTestBackend(t, map[string]constant.Proxy{
		first.Name(): first, second.Name(): second, third.Name(): third, "duplicate": other,
	})
	for _, group := range []constant.Proxy{first, second, third} {
		snapshot, _ := backend.Snapshot(group.Name())
		healthy, err := backend.Probe(backend.ctx, snapshot, snapshot.Current)
		if err != nil || healthy != (group != third) {
			t.Fatalf("%s: healthy = %v, err = %v", group.Name(), healthy, err)
		}
	}
	if actual.calls.Load() != 1 || other.calls.Load() != 1 {
		t.Fatalf("actual calls = %d, other calls = %d", actual.calls.Load(), other.calls.Load())
	}
}

func TestStickyBackendDoesNotCacheUnavailableProbe(t *testing.T) {
	node := stickyNode("a")
	group := stickyTestGroup(t, "[Sticky] test", node)
	backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
	snapshot, _ := backend.Snapshot(group.Name())
	for i := 0; i < cap(backend.slots); i++ {
		backend.slots <- struct{}{}
	}
	subscriber := log.Subscribe()
	defer log.UnSubscribe(subscriber)
	started := time.Now()
	if healthy, err := backend.Probe(backend.ctx, snapshot, "a"); healthy || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("saturated probe healthy = %v, err = %v", healthy, err)
	}
	if elapsed := time.Since(started); elapsed < 200*time.Millisecond || elapsed > time.Second {
		t.Fatalf("queue wait = %v, want approximately 250ms", elapsed)
	}
	if node.calls.Load() != 0 {
		t.Fatal("queue timeout must not call URLTest")
	}
	if drainLogStream(t, subscriber, 100*time.Millisecond, func(payload string) bool {
		return strings.Contains(payload, "[Sticky] probe failed:")
	}) {
		t.Fatal("queue timeout was logged as a node failure")
	}
	<-backend.slots
	if healthy, err := backend.Probe(backend.ctx, snapshot, "a"); err != nil || !healthy {
		t.Fatalf("retry healthy = %v, err = %v", healthy, err)
	}
	if node.calls.Load() != 1 {
		t.Fatalf("actual probes = %d, want 1", node.calls.Load())
	}
}

func TestStickyBackendConfirmsFailuresWithFreshProbes(t *testing.T) {
	for _, recovers := range []bool{false, true} {
		t.Run(fmt.Sprintf("recovers=%t", recovers), func(t *testing.T) {
			current := stickyNode("a")
			current.healthy = false
			group := stickyTestGroup(t, "[Sticky] test", current, stickyNode("b"))
			backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
			snapshot, _ := backend.Snapshot(group.Name())
			var failures stickyFailureState
			assertOutcome(t, stickyStep(backend.ctx, backend, snapshot, &failures), stickyFailurePending)
			if groupNow(t, group) != "a" {
				t.Fatal("first failure changed selection")
			}
			current.healthy = recovers
			want := stickyCommitted
			wantNode := "b"
			if recovers {
				want = stickyCurrentHealthy
				wantNode = "a"
			}
			assertOutcome(t, stickyStep(backend.ctx, backend, snapshot, &failures), want)
			if current.calls.Load() != 2 {
				t.Fatalf("current probes = %d, want two independent requests", current.calls.Load())
			}
			if got := groupNow(t, group); got != wantNode {
				t.Fatalf("selected %s, want %s", got, wantNode)
			}
		})
	}
}

func TestStickyBackendRequestDeadlineExcludesQueueWait(t *testing.T) {
	for _, queueWait := range []time.Duration{0, 120 * time.Millisecond} {
		t.Run(queueWait.String(), func(t *testing.T) {
			node := stickyNode("a")
			var requestStarted, deadline time.Time
			node.onProbe = func(ctx context.Context) {
				requestStarted = time.Now()
				var ok bool
				deadline, ok = ctx.Deadline()
				if !ok {
					t.Error("URLTest context has no deadline")
				}
			}
			group := stickyTestGroup(t, "[Sticky] test", node)
			backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
			snapshot, _ := backend.Snapshot(group.Name())
			ctx, cancel := context.WithTimeout(backend.ctx, 20*time.Second)
			defer cancel()
			if queueWait > 0 {
				for i := 0; i < cap(backend.slots); i++ {
					backend.slots <- struct{}{}
				}
				timer := time.AfterFunc(queueWait, func() { <-backend.slots })
				defer timer.Stop()
			}
			started := time.Now()
			if healthy, err := backend.Probe(ctx, snapshot, "a"); err != nil || !healthy {
				t.Fatalf("healthy = %v, err = %v", healthy, err)
			}
			if budget := deadline.Sub(requestStarted); budget < 5*time.Second-50*time.Millisecond || budget > 5*time.Second {
				t.Fatalf("URLTest budget = %v, want a separate five-second deadline", budget)
			}
			if queueWait > 0 && requestStarted.Sub(started) < 100*time.Millisecond {
				t.Fatal("URLTest did not wait for the occupied slot")
			}
		})
	}
}

func TestStickyBackendSlowSuccessfulProbeStaysHealthy(t *testing.T) {
	node := stickyNode("a")
	node.onProbe = func(context.Context) { time.Sleep(2100 * time.Millisecond) }
	group := stickyTestGroup(t, "[Sticky] test", node)
	backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
	snapshot, _ := backend.Snapshot(group.Name())
	subscriber := log.Subscribe()
	defer log.UnSubscribe(subscriber)
	if healthy, err := backend.Probe(backend.ctx, snapshot, "a"); err != nil || !healthy {
		t.Fatalf("2.1-second probe healthy = %v, err = %v", healthy, err)
	}
	if drainLogStream(t, subscriber, 100*time.Millisecond, func(payload string) bool {
		return strings.Contains(payload, "[Sticky]")
	}) {
		t.Fatal("healthy probes must not publish Sticky log events")
	}
}

func TestStickyBackendExpiredRequestWithNilURLTestErrorIsUnhealthy(t *testing.T) {
	node := stickyNode("a")
	node.probeResult = func(ctx context.Context) (uint16, error) {
		<-ctx.Done()
		return 42, nil
	}
	group := stickyTestGroup(t, "[Sticky] test", node)
	backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
	snapshot, _ := backend.Snapshot(group.Name())
	ctx, cancel := context.WithTimeout(backend.ctx, 10*time.Second)
	defer cancel()
	subscriber := log.Subscribe()
	defer log.UnSubscribe(subscriber)
	if healthy, err := backend.Probe(ctx, snapshot, "a"); err != nil || healthy {
		t.Fatalf("expired request healthy = %v, err = %v", healthy, err)
	}
	if ctx.Err() != nil {
		t.Fatalf("parent context expired: %v", ctx.Err())
	}
	if !drainLogStream(t, subscriber, time.Second, func(payload string) bool {
		return strings.Contains(payload, "[Sticky] probe failed:") &&
			strings.Contains(payload, "delay=42 urltest_err=<nil> deadline_err=context deadline exceeded")
	}) {
		t.Fatal("request deadline failure was not logged with the actual nil URLTest error")
	}
}

func TestStickyBackendLogsRealProbeFailure(t *testing.T) {
	node := stickyNode("a")
	node.probeResult = func(context.Context) (uint16, error) {
		time.Sleep(30 * time.Millisecond)
		return 7, errors.New("test URLTest failure")
	}
	group := stickyTestGroup(t, "[Sticky] test", node)
	backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
	snapshot, _ := backend.Snapshot(group.Name())
	subscriber := log.Subscribe()
	defer log.UnSubscribe(subscriber)
	if healthy, err := backend.Probe(backend.ctx, snapshot, "a"); err != nil || healthy {
		t.Fatalf("failed request healthy = %v, err = %v", healthy, err)
	}
	var payload string
	if !drainLogStream(t, subscriber, time.Second, func(event string) bool {
		if strings.Contains(event, "[Sticky] probe failed:") {
			payload = event
			return true
		}
		return false
	}) {
		t.Fatal("real probe failure was not logged")
	}
	for _, field := range []string{"node=\"a\"", "url=" + stickyURL, "delay=7", "urltest_err=test URLTest failure", "deadline_err=<nil>"} {
		if !strings.Contains(payload, field) {
			t.Errorf("log missing %q: %s", field, payload)
		}
	}
	_, elapsedField, _ := strings.Cut(payload, "elapsed=")
	elapsedText, _, _ := strings.Cut(elapsedField, " ")
	elapsed, err := time.ParseDuration(elapsedText)
	if err != nil || elapsed < 30*time.Millisecond {
		t.Fatalf("logged elapsed = %q, parse error = %v; want full request duration", elapsedText, err)
	}
}

func TestStickyBackendPreservesParentContextErrorsWithoutFailureLog(t *testing.T) {
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		for _, returnNil := range []bool{false, true} {
			name := want.Error() + "/URLTest-error"
			if returnNil {
				name = want.Error() + "/URLTest-nil"
			}
			t.Run(name, func(t *testing.T) {
				node := stickyNode("a")
				group := stickyTestGroup(t, "[Sticky] test", node)
				backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
				snapshot, _ := backend.Snapshot(group.Name())
				var ctx context.Context
				var cancel context.CancelFunc
				if want == context.Canceled {
					ctx, cancel = context.WithCancel(backend.ctx)
				} else {
					ctx, cancel = context.WithTimeout(backend.ctx, 30*time.Millisecond)
				}
				defer cancel()
				node.probeResult = func(probeCtx context.Context) (uint16, error) {
					if want == context.Canceled {
						cancel()
					}
					<-probeCtx.Done()
					if returnNil {
						return 10, nil
					}
					return 0, errors.New("URLTest interrupted")
				}
				subscriber := log.Subscribe()
				defer log.UnSubscribe(subscriber)
				if healthy, err := backend.Probe(ctx, snapshot, "a"); healthy || !errors.Is(err, want) {
					t.Fatalf("healthy = %v, err = %v, want %v", healthy, err, want)
				}
				if drainLogStream(t, subscriber, 100*time.Millisecond, func(payload string) bool {
					return strings.Contains(payload, "[Sticky] probe failed:")
				}) {
					t.Fatal("parent context cancellation was logged as a node failure")
				}
				if _, cached := backend.probes[node]; cached {
					t.Fatal("parent context error must not be cached")
				}
			})
		}
	}
}

func TestStickyBackendQueueCancellationPreservesParentError(t *testing.T) {
	node := stickyNode("a")
	group := stickyTestGroup(t, "[Sticky] test", node)
	backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
	snapshot, _ := backend.Snapshot(group.Name())
	for i := 0; i < cap(backend.slots); i++ {
		backend.slots <- struct{}{}
	}
	ctx, cancel := context.WithCancel(backend.ctx)
	defer cancel()
	timer := time.AfterFunc(30*time.Millisecond, cancel)
	defer timer.Stop()
	subscriber := log.Subscribe()
	defer log.UnSubscribe(subscriber)
	if healthy, err := backend.Probe(ctx, snapshot, "a"); healthy || !errors.Is(err, context.Canceled) {
		t.Fatalf("queued cancellation healthy = %v, err = %v", healthy, err)
	}
	if node.calls.Load() != 0 {
		t.Fatal("canceled queued probe called URLTest")
	}
	if drainLogStream(t, subscriber, 100*time.Millisecond, func(payload string) bool {
		return strings.Contains(payload, "[Sticky] probe failed:")
	}) {
		t.Fatal("queue cancellation was logged as a node failure")
	}
}

func TestStickyBackendLogsOnlyCommittedSwitch(t *testing.T) {
	group := stickyTestGroup(t, "[Sticky] test", stickyNode("a"), stickyNode("b"))
	backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
	snapshot, _ := backend.Snapshot(group.Name())
	subscriber := log.Subscribe()
	defer log.UnSubscribe(subscriber)
	if backend.Commit(snapshot, "missing") {
		t.Fatal("invalid candidate was committed")
	}
	if drainLogStream(t, subscriber, 100*time.Millisecond, func(payload string) bool {
		return strings.Contains(payload, "[Sticky] switch:")
	}) {
		t.Fatal("rejected commit published a switch log")
	}
	if !backend.Commit(snapshot, "b") {
		t.Fatal("valid candidate was not committed")
	}
	if !drainLogStream(t, subscriber, time.Second, func(payload string) bool {
		return strings.Contains(payload, "[Sticky] switch: group=\"[Sticky] test\" \"a\" -> \"b\"")
	}) {
		t.Fatal("committed switch was not logged with the old and new selections")
	}
	snapshot, _ = backend.Snapshot(group.Name())
	if !backend.Commit(snapshot, "b") {
		t.Fatal("unchanged selection was rejected")
	}
	if drainLogStream(t, subscriber, 100*time.Millisecond, func(payload string) bool {
		return strings.Contains(payload, "[Sticky] switch:")
	}) {
		t.Fatal("unchanged selection published a switch log")
	}
}

func TestStickyBackendRestoresPersistedChoiceOverStaleFlutterMap(t *testing.T) {
	group := stickyTestGroup(t, "[Sticky] test", stickyNode("a"), stickyNode("b"))
	backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
	snapshot, _ := backend.Snapshot(group.Name())
	if !backend.Commit(snapshot, "b") {
		t.Fatal("could not commit healthy candidate")
	}
	stopSticky()
	stickySelections = nil
	patchSelectGroup(map[string]string{group.Name(): "a"})
	configureSticky([]byte("profile-a"), true)
	if got := groupNow(t, group); got != "b" {
		t.Fatalf("restored %s, want b", got)
	}
	patchSelectGroup(map[string]string{group.Name(): "a"})
	configureSticky([]byte("profile-b"), true)
	if got := groupNow(t, group); got != "a" {
		t.Fatalf("another config inherited %s", got)
	}
}

func TestStickyBackendManualChoiceDuringApplyWinsWithoutCrossScopeWrite(t *testing.T) {
	group := stickyTestGroup(t, "[Sticky] test", stickyNode("a"), stickyNode("b"), stickyNode("c"))
	stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
	if err := handleChangeProxy(&ChangeProxyParams{GroupName: group.Name(), ProxyName: "b"}); err != "" {
		t.Fatal(err)
	}
	oldScope := stickyScope
	beginStickyConfig()
	defer endStickyConfig()
	if err := handleChangeProxy(&ChangeProxyParams{GroupName: group.Name(), ProxyName: "c"}); err != "" {
		t.Fatal(err)
	}
	patchSelectGroup(map[string]string{group.Name(): "a"})
	configureSticky([]byte("profile-b"), true)
	if got := groupNow(t, group); got != "c" {
		t.Fatalf("manual choice lost during apply: %s", got)
	}
	if stickySelections[oldScope][group.Name()] != "b" || stickySelections[stickyScope][group.Name()] != "c" {
		t.Fatal("manual choice was persisted to the wrong config scope")
	}
}

func TestStickyBackendStopCancelsActiveProbe(t *testing.T) {
	node := stickyNode("a")
	node.onProbe = func(ctx context.Context) {
		stopSticky()
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Error("probe did not receive cancellation")
		}
	}
	group := stickyTestGroup(t, "[Sticky] test", node, stickyNode("b"))
	backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
	snapshot, _ := backend.Snapshot(group.Name())
	if _, err := backend.Probe(backend.ctx, snapshot, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("probe error = %v, want context.Canceled", err)
	}
	if backend.Commit(snapshot, "b") {
		t.Fatal("canceled probe committed a candidate")
	}
	if _, err := os.Stat(filepath.Join(constant.Path.HomeDir(), "sticky-selections.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled probe persisted a choice: %v", err)
	}
}
