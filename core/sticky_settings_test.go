package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/metacubex/mihomo/config"
	"github.com/metacubex/mihomo/constant"
)

func TestStickySettingsDefaultsAndValidation(t *testing.T) {
	for _, tc := range []struct {
		url         string
		seconds     int
		wantURL     string
		wantTimeout time.Duration
	}{
		{"", 0, stickyURL, stickyTimeout},
		{"ftp://example.com", -1, stickyURL, stickyTimeout},
		{"https://", 61, stickyURL, stickyTimeout},
		{"https://user:password@example.com", 5, stickyURL, stickyTimeout},
		{" https://example.com/204 ", 1, "https://example.com/204", time.Second},
		{"http://localhost:8080/check", 60, "http://localhost:8080/check", time.Minute},
	} {
		got := normalizeStickySettings(tc.url, tc.seconds, 0)
		if got.URL != tc.wantURL || got.Timeout != tc.wantTimeout {
			t.Errorf("normalizeStickySettings(%q, %d) = %+v", tc.url, tc.seconds, got)
		}
	}
}

func TestStickySettingsSetupProtocol(t *testing.T) {
	params := defaultSetupParams()
	if err := json.Unmarshal([]byte(`{"test-url":"https://manual.example/204","sticky-test-url":"https://health.example/204","sticky-timeout":12}`), params); err != nil {
		t.Fatal(err)
	}
	if params.StickyTestURL != "https://health.example/204" || params.StickyTimeout != 12 {
		t.Fatalf("decoded setup = %+v", params)
	}
	legacy := defaultSetupParams()
	if err := json.Unmarshal([]byte(`{"selected-map":{}}`), legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.StickyTestURL != stickyURL || time.Duration(legacy.StickyTimeout)*time.Second != stickyTimeout {
		t.Fatalf("legacy setup = %+v", legacy)
	}
}

func TestStickyConfiguredFailureThreshold(t *testing.T) {
	for threshold := 1; threshold <= 10; threshold++ {
		fake := newStickyFake("a", "a", "b")
		fake.failureThreshold = threshold
		fake.healthy["b"] = true
		var failures stickyFailureState
		for attempt := 1; attempt < threshold; attempt++ {
			assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyFailurePending)
			assertCommitted(t, fake, nil)
		}
		assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyCommitted)
		assertCommitted(t, fake, []string{"b"})
	}
}

func TestStickyConfiguredFailuresMustBeConsecutive(t *testing.T) {
	fake := newStickyFake("a", "a", "b")
	fake.failureThreshold = 3
	fake.healthy["b"] = true
	var failures stickyFailureState
	for attempt := 0; attempt < 2; attempt++ {
		assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyFailurePending)
	}
	fake.healthy["a"] = true
	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyCurrentHealthy)
	fake.healthy["a"] = false
	for attempt := 0; attempt < 2; attempt++ {
		assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyFailurePending)
	}
	assertCommitted(t, fake, nil)
	assertOutcome(t, stickyStep(context.Background(), fake, fake.snapshot, &failures), stickyCommitted)
}

func TestStickyThresholdValidation(t *testing.T) {
	for _, threshold := range []int{-1, 0, 11} {
		if got := normalizeStickySettings("", 0, threshold).FailureThreshold; got != 2 {
			t.Errorf("threshold %d normalized to %d", threshold, got)
		}
	}
	for threshold := 1; threshold <= 10; threshold++ {
		if got := normalizeStickySettings("", 0, threshold).FailureThreshold; got != threshold {
			t.Errorf("threshold %d normalized to %d", threshold, got)
		}
	}
}

func TestStickyProbeUsesConfiguredURLAndDeadline(t *testing.T) {
	for _, seconds := range []int{1, 12, 60} {
		node := stickyNode("a")
		group := stickyTestGroup(t, "[Sticky] test", node)
		backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
		backend.settings = normalizeStickySettings("https://health.example/204", seconds, 2)
		node.onURLTest = func(ctx context.Context, url string) {
			if url != backend.settings.URL {
				t.Errorf("URLTest URL = %q", url)
			}
			deadline, ok := ctx.Deadline()
			remaining := time.Until(deadline)
			want := time.Duration(seconds) * time.Second
			if !ok || remaining > want || remaining < want-time.Second {
				t.Errorf("URLTest remaining deadline = %s, want %s", remaining, want)
			}
		}
		snapshot, _ := backend.Snapshot(group.Name())
		healthy, err := backend.Probe(backend.ctx, snapshot, node.Name())
		if err != nil || !healthy || node.calls.Load() != 1 {
			t.Fatalf("probe = %v, %v, calls = %d", healthy, err, node.calls.Load())
		}
		stopSticky()
	}
}

func TestStickySettingsUpdateCancelsOldRunAndPreservesSelection(t *testing.T) {
	group := stickyTestGroup(t, "[Sticky] test", stickyNode("a"), stickyNode("b"))
	backend := stickyTestBackend(t, map[string]constant.Proxy{group.Name(): group})
	previousSettings, previousConfig := stickySettings, currentConfig
	t.Cleanup(func() {
		stopSticky()
		stickySettings, currentConfig = previousSettings, previousConfig
	})
	stickySettings = backend.settings
	currentConfig = &config.Config{}
	snapshot, _ := backend.Snapshot(group.Name())
	url, seconds := "https://health.example/204", 12
	threshold := 1
	syncStickySettings(&url, &seconds, &threshold)
	next := stickyRun
	if backend.ctx.Err() == nil || next == nil || next == backend {
		t.Fatal("settings update must replace and cancel the old run")
	}
	if next.settings.URL != url || next.settings.Timeout != 12*time.Second || next.settings.FailureThreshold != 1 {
		t.Fatalf("new run settings = %+v", next.settings)
	}
	if backend.Commit(snapshot, "b") {
		t.Fatal("old run committed a stale selection")
	}
	selector, _ := stickySelector(group)
	if selector.Now() != "a" {
		t.Fatalf("selection = %q, want a", selector.Now())
	}
	syncStickySettings(&url, &seconds, &threshold)
	if stickyRun != next {
		t.Fatal("unchanged settings restarted the run")
	}
	stopSticky()
	isRunning.Store(false)
	seconds = 3
	syncStickySettings(nil, &seconds, nil)
	if stickyRun != nil || stickySettings.Timeout != 3*time.Second || stickySettings.URL != url {
		t.Fatal("stopped core must retain updated settings without starting probes")
	}
}
