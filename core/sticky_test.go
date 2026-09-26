package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outboundgroup"
	"github.com/metacubex/mihomo/adapter/provider"
	"github.com/metacubex/mihomo/common/utils"
	"github.com/metacubex/mihomo/constant"
	cp "github.com/metacubex/mihomo/constant/provider"
	"github.com/metacubex/mihomo/tunnel"
)

type stickyTestNode struct {
	constant.Proxy
	calls   atomic.Int32
	healthy bool
	onProbe func(context.Context)
}

func (p *stickyTestNode) Type() constant.AdapterType { return constant.Shadowsocks }

func (p *stickyTestNode) URLTest(ctx context.Context, _ string, _ utils.IntRanges[uint16]) (uint16, error) {
	p.calls.Add(1)
	if p.onProbe != nil {
		p.onProbe(ctx)
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
	if _, err := backend.Probe(backend.ctx, snapshot, "a"); err == nil {
		t.Fatal("saturated probe should report unavailable")
	}
	<-backend.slots
	if healthy, err := backend.Probe(backend.ctx, snapshot, "a"); err != nil || !healthy {
		t.Fatalf("retry healthy = %v, err = %v", healthy, err)
	}
	if node.calls.Load() != 1 {
		t.Fatalf("actual probes = %d, want 1", node.calls.Load())
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
