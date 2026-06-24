package health

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"light-api-gateway/internal/config"
)

func TestCheckOnceUpdatesTargetStatuses(t *testing.T) {
	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer healthyServer.Close()

	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unhealthyServer.Close()

	store := &fakeStore{
		targets: []config.TargetConfig{
			{ID: "target-healthy", URL: healthyServer.URL, Enabled: true, HealthStatus: StatusUnknown},
			{ID: "target-unhealthy", URL: unhealthyServer.URL, Enabled: true, HealthStatus: StatusHealthy},
			{ID: "target-disabled", URL: "http://127.0.0.1:1", Enabled: false, HealthStatus: StatusUnknown},
		},
	}
	checker := NewChecker(store, slog.New(slog.NewTextHandler(testWriter{t: t}, nil)))

	checker.CheckOnce(context.Background())

	if got := store.status("target-healthy"); got != StatusHealthy {
		t.Fatalf("healthy target status = %q, want %q", got, StatusHealthy)
	}
	if got := store.status("target-unhealthy"); got != StatusUnhealthy {
		t.Fatalf("unhealthy target status = %q, want %q", got, StatusUnhealthy)
	}
	if got := store.failures("target-unhealthy"); got != 1 {
		t.Fatalf("unhealthy target failures = %d, want 1", got)
	}
	if got := store.status("target-disabled"); got != StatusUnknown {
		t.Fatalf("disabled target status = %q, want %q", got, StatusUnknown)
	}
}

func TestCheckOnceAutoDisablesAfterFailureThreshold(t *testing.T) {
	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unhealthyServer.Close()

	store := &fakeStore{
		targets: []config.TargetConfig{
			{ID: "target-unhealthy", URL: unhealthyServer.URL, Enabled: true, HealthStatus: StatusHealthy},
		},
	}
	checker := NewChecker(
		store,
		slog.New(slog.NewTextHandler(testWriter{t: t}, nil)),
		WithAutoDisableThreshold(2),
	)

	checker.CheckOnce(context.Background())
	if !store.enabled("target-unhealthy") {
		t.Fatal("target disabled before failure threshold")
	}
	if got := store.failures("target-unhealthy"); got != 1 {
		t.Fatalf("failures after first check = %d, want 1", got)
	}

	checker.CheckOnce(context.Background())
	if store.enabled("target-unhealthy") {
		t.Fatal("target still enabled after failure threshold")
	}
	if got := store.failures("target-unhealthy"); got != 2 {
		t.Fatalf("failures after second check = %d, want 2", got)
	}

	checker.CheckOnce(context.Background())
	if got := store.failures("target-unhealthy"); got != 2 {
		t.Fatalf("disabled target failures changed to %d, want 2", got)
	}
}

type fakeStore struct {
	mu      sync.Mutex
	targets []config.TargetConfig
}

func (s *fakeStore) ListAllUpstreamTargets(ctx context.Context) ([]config.TargetConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	targets := make([]config.TargetConfig, len(s.targets))
	copy(targets, s.targets)
	return targets, nil
}

func (s *fakeStore) SetUpstreamTargetHealthStatus(ctx context.Context, id string, status string) (config.TargetConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.targets {
		if s.targets[i].ID == id {
			s.targets[i].HealthStatus = status
			return s.targets[i], nil
		}
	}
	return config.TargetConfig{}, nil
}

func (s *fakeStore) RecordUpstreamTargetHealthCheck(ctx context.Context, id string, status string, failed bool, autoDisableThreshold int) (config.TargetConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.targets {
		if s.targets[i].ID != id {
			continue
		}
		s.targets[i].HealthStatus = status
		if failed {
			s.targets[i].ConsecutiveFailures++
			if autoDisableThreshold > 0 && s.targets[i].ConsecutiveFailures >= autoDisableThreshold {
				s.targets[i].Enabled = false
			}
		} else {
			s.targets[i].ConsecutiveFailures = 0
		}
		return s.targets[i], nil
	}
	return config.TargetConfig{}, nil
}

func (s *fakeStore) status(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, target := range s.targets {
		if target.ID == id {
			return target.HealthStatus
		}
	}
	return ""
}

func (s *fakeStore) enabled(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, target := range s.targets {
		if target.ID == id {
			return target.Enabled
		}
	}
	return false
}

func (s *fakeStore) failures(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, target := range s.targets {
		if target.ID == id {
			return target.ConsecutiveFailures
		}
	}
	return 0
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
