package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outboundgroup"
	"github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel"
)

const (
	stickyPrefix     = "[Sticky] "
	stickyURL        = "https://cp.cloudflare.com"
	stickyInterval   = time.Second
	stickyTimeout    = 2 * time.Second
	stickyRetryDelay = 5 * time.Second
)

var (
	stickyRun            *mihomoStickyBackend
	stickyScope          string
	stickyPath           string
	stickySelections     map[string]map[string]string
	stickyApplying       bool
	stickyPending        map[string]string
	errStickyUnavailable = errors.New("sticky probe unavailable")
)

type stickyView struct {
	proxy    constant.Proxy
	members  []constant.Proxy
	current  string
	revision uint64
}

type stickyProbe struct {
	done    chan struct{}
	expires time.Time
	healthy bool
	err     error
}

type mihomoStickyBackend struct {
	ctx     context.Context
	cancel  context.CancelFunc
	views   map[string]stickyView
	probeMu sync.Mutex
	probes  map[constant.Proxy]*stickyProbe
	slots   chan struct{}
}

func isStickyName(name string) bool {
	return strings.HasPrefix(name, stickyPrefix)
}

func stickySelector(proxy constant.Proxy) (*outboundgroup.Selector, bool) {
	if proxy == nil || !isStickyName(proxy.Name()) {
		return nil, false
	}
	p, ok := proxy.(*adapter.Proxy)
	if !ok {
		return nil, false
	}
	s, ok := p.ProxyAdapter.(*outboundgroup.Selector)
	return s, ok
}

func stickyLeaf(proxy constant.Proxy) bool {
	if proxy == nil || isProxyGroupType(proxy.Type()) {
		return false
	}
	switch proxy.Type() {
	case constant.Direct, constant.Reject, constant.RejectDrop, constant.Dns,
		constant.Pass, constant.PassRule, constant.Rematch, constant.Compatible:
		return false
	default:
		return true
	}
}

// configMu owns setup/start/stop; selectMu protects the runtime and persisted selection view.
func stopSticky() {
	selectMu.Lock()
	defer selectMu.Unlock()
	if stickyRun != nil {
		stickyRun.cancel()
		stickyRun = nil
	}
}

func beginStickyConfig() {
	stopSticky()
	selectMu.Lock()
	defer selectMu.Unlock()
	stickyApplying = true
	stickyPending = make(map[string]string)
}

func endStickyConfig() {
	selectMu.Lock()
	defer selectMu.Unlock()
	stickyApplying = false
	stickyPending = nil
}

func configureSticky(data []byte, valid bool) {
	selectMu.Lock()
	defer selectMu.Unlock()
	stickyApplying = false
	pending := stickyPending
	stickyPending = nil
	stickyScope = ""
	if !valid {
		return
	}
	digest := sha256.Sum256(data)
	stickyScope = hex.EncodeToString(digest[:])
	path := filepath.Join(constant.Path.HomeDir(), "sticky-selections.json")
	if stickyPath != path || stickySelections == nil {
		stickyPath = path
		stickySelections = make(map[string]map[string]string)
		if data, err := os.ReadFile(path); err == nil {
			if err := json.Unmarshal(data, &stickySelections); err != nil {
				logError("sticky selection restore: %v", err)
				stickySelections = make(map[string]map[string]string)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			logError("sticky selection read: %v", err)
		}
		if stickySelections == nil {
			stickySelections = make(map[string]map[string]string)
		}
	}
	for name, selected := range stickySelections[stickyScope] {
		if selector, ok := stickySelector(lookupProxy(name)); ok {
			selector.ForceSet(selected)
		}
	}
	for name, selected := range pending {
		if selector, ok := stickySelector(lookupProxy(name)); ok && selector.Set(selected) == nil {
			recordStickySelection(name)
		}
	}
}

func reconcileSticky() {
	stopSticky()
	if currentConfig == nil || !isRunning.Load() || isSuspended.Load() {
		return
	}
	selectMu.Lock()
	defer selectMu.Unlock()
	if stickyScope == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	backend := &mihomoStickyBackend{
		ctx: ctx, cancel: cancel,
		views:  make(map[string]stickyView),
		probes: make(map[constant.Proxy]*stickyProbe),
		slots:  make(chan struct{}, 16),
	}
	stickyRun = backend
	for name, proxy := range tunnel.AllProxies() {
		if _, ok := stickySelector(proxy); !ok {
			continue
		}
		group := name
		safeGoDetached("stickyFailover", func() {
			runStickyGroup(ctx, backend, group, stickyInterval, stickyRetryDelay)
		})
	}
}

func (b *mihomoStickyBackend) snapshotLocked(group string) (stickySnapshot, bool) {
	if stickyRun != b || b.ctx.Err() != nil || !isRunning.Load() || isSuspended.Load() {
		return stickySnapshot{}, false
	}
	proxy := lookupProxy(group)
	selector, ok := stickySelector(proxy)
	if !ok {
		return stickySnapshot{}, false
	}
	current := selector.Now()
	members := make([]constant.Proxy, 0)
	names := make([]string, 0)
	currentSupported := false
	seen := make(map[string]bool)
	for _, member := range selector.Proxies() {
		if seen[member.Name()] {
			continue
		}
		seen[member.Name()] = true
		if !stickyLeaf(member) {
			continue
		}
		members = append(members, member)
		names = append(names, member.Name())
		currentSupported = currentSupported || member.Name() == current
	}
	previous := b.views[group]
	if previous.proxy != proxy || previous.current != current || !slices.Equal(previous.members, members) {
		previous = stickyView{proxy: proxy, members: members, current: current, revision: previous.revision + 1}
		b.views[group] = previous
	}
	return stickySnapshot{Group: group, Current: current, Members: names, Revision: previous.revision}, currentSupported
}

func (b *mihomoStickyBackend) Snapshot(group string) (stickySnapshot, bool) {
	selectMu.Lock()
	defer selectMu.Unlock()
	return b.snapshotLocked(group)
}

func (b *mihomoStickyBackend) Probe(ctx context.Context, snapshot stickySnapshot, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	selectMu.Lock()
	current, ok := b.snapshotLocked(snapshot.Group)
	var proxy constant.Proxy
	if ok && current.Revision == snapshot.Revision && current.Current == snapshot.Current {
		for _, member := range b.views[snapshot.Group].members {
			if member.Name() == name {
				proxy = member
				break
			}
		}
	}
	selectMu.Unlock()
	if proxy == nil {
		return false, errStickyUnavailable
	}
	b.probeMu.Lock()
	probe, exists := b.probes[proxy]
	if exists {
		select {
		case <-probe.done:
			exists = time.Now().Before(probe.expires)
		default:
		}
	} else {
		exists = false
	}
	if !exists {
		probe = &stickyProbe{done: make(chan struct{}), expires: time.Now().Add(stickyInterval)}
		for key, previous := range b.probes {
			if time.Now().Before(previous.expires) {
				continue
			}
			select {
			case <-previous.done:
				delete(b.probes, key)
			default:
			}
		}
		b.probes[proxy] = probe
	}
	b.probeMu.Unlock()
	if exists {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-probe.done:
			return probe.healthy, probe.err
		}
	}
	defer close(probe.done)
	probe.healthy, probe.err = b.probeNode(ctx, proxy)
	if probe.err != nil {
		b.probeMu.Lock()
		if b.probes[proxy] == probe {
			delete(b.probes, proxy)
		}
		b.probeMu.Unlock()
	}
	return probe.healthy, probe.err
}

func (b *mihomoStickyBackend) probeNode(ctx context.Context, proxy constant.Proxy) (bool, error) {
	queueCtx, cancelQueue := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancelQueue()
	select {
	case b.slots <- struct{}{}:
		defer func() { <-b.slots }()
	case <-queueCtx.Done():
		return false, queueCtx.Err()
	}
	probeCtx, cancel := context.WithTimeout(ctx, stickyTimeout)
	defer cancel()
	_, err := proxy.URLTest(probeCtx, stickyURL, anyDelayTestStatus)
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	return err == nil && probeCtx.Err() == nil, nil
}

func (b *mihomoStickyBackend) Commit(snapshot stickySnapshot, candidate string) bool {
	selectMu.Lock()
	defer selectMu.Unlock()
	current, ok := b.snapshotLocked(snapshot.Group)
	if !ok || current.Revision != snapshot.Revision || current.Current != snapshot.Current || !slices.Contains(current.Members, candidate) {
		return false
	}
	selector, ok := stickySelector(lookupProxy(snapshot.Group))
	if !ok || selector.Set(candidate) != nil {
		return false
	}
	recordStickySelection(snapshot.Group)
	return true
}

func recordStickySelection(group string) {
	selector, ok := stickySelector(lookupProxy(group))
	if !ok {
		return
	}
	if stickyApplying {
		stickyPending[group] = selector.Now()
		return
	}
	if stickyScope == "" {
		return
	}
	if stickyRun != nil {
		view := stickyRun.views[group]
		view.revision++
		stickyRun.views[group] = view
	}
	if stickySelections[stickyScope] == nil {
		stickySelections[stickyScope] = make(map[string]string)
	}
	stickySelections[stickyScope][group] = selector.Now()
	data, err := json.Marshal(stickySelections)
	if err == nil {
		err = os.WriteFile(stickyPath+".tmp", data, 0600)
	}
	if err == nil {
		err = os.Rename(stickyPath+".tmp", stickyPath)
	}
	if err != nil {
		logError("sticky selection save: %v", err)
	}
}
