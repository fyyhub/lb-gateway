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
	if got := store.status("target-disabled"); got != StatusUnknown {
		t.Fatalf("disabled target status = %q, want %q", got, StatusUnknown)
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

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
