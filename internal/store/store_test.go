package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"light-api-gateway/internal/config"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()

	st, err := Open(filepath.Join(t.TempDir(), "gateway.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		st.Close()
	})

	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}

	return st
}

func TestSeedConfigAndLoadRuntimeConfig(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	cfg := config.Config{
		Gateway: config.GatewayConfig{Listen: ":8080", LogRequests: true},
		Routes: []config.RouteConfig{
			{
				Name:     "api-route",
				Enabled:  true,
				Priority: 100,
				Type:     "proxy",
				Match:    config.MatchConfig{Path: "/api/**", Methods: []string{"GET"}},
				UpstreamGroup: config.UpstreamGroup{
					Strategy: "weighted-round-robin",
					Targets: []config.TargetConfig{
						{URL: "http://127.0.0.1:9001", Weight: 2, Enabled: true},
						{URL: "http://127.0.0.1:9002", Weight: 1, Enabled: true},
					},
				},
			},
			{
				Name:     "web-redirect",
				Enabled:  true,
				Priority: 90,
				Type:     "redirect",
				Match:    config.MatchConfig{Path: "/web", Methods: []string{"GET"}},
				Redirect: &config.RedirectConfig{
					StatusCode: 302,
					Strategy:   "round-robin",
					Targets: []config.TargetConfig{
						{URL: "https://example.com", Weight: 1, Enabled: true},
					},
				},
			},
		},
	}

	if err := st.SeedConfig(ctx, cfg); err != nil {
		t.Fatalf("SeedConfig returned error: %v", err)
	}

	loaded, err := st.LoadConfig(ctx, cfg.Gateway)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if len(loaded.Routes) != 2 {
		t.Fatalf("got %d routes, want 2", len(loaded.Routes))
	}
	if len(loaded.Routes[0].UpstreamGroup.Targets) != 2 {
		t.Fatalf("got %d upstream targets, want 2", len(loaded.Routes[0].UpstreamGroup.Targets))
	}
}

func TestRouteCRUD(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	route, err := st.CreateRoute(ctx, config.RouteConfig{
		Name:     "web",
		Enabled:  true,
		Priority: 10,
		Type:     "redirect",
		Match:    config.MatchConfig{Path: "/web"},
		Redirect: &config.RedirectConfig{
			StatusCode: 302,
			Targets:    []config.TargetConfig{{URL: "https://example.com", Enabled: true}},
		},
	})
	if err != nil {
		t.Fatalf("CreateRoute returned error: %v", err)
	}

	route, err = st.SetRouteEnabled(ctx, route.ID, false)
	if err != nil {
		t.Fatalf("SetRouteEnabled returned error: %v", err)
	}
	if route.Enabled {
		t.Fatal("expected route to be disabled")
	}

	route.Priority = 20
	updated, err := st.UpdateRoute(ctx, route.ID, route)
	if err != nil {
		t.Fatalf("UpdateRoute returned error: %v", err)
	}
	if updated.Priority != 20 {
		t.Fatalf("got priority %d, want 20", updated.Priority)
	}

	if err := st.DeleteRoute(ctx, route.ID); err != nil {
		t.Fatalf("DeleteRoute returned error: %v", err)
	}
}

func TestUpstreamCRUD(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	group, err := st.CreateUpstreamGroup(ctx, config.UpstreamGroup{
		Name:     "api",
		Strategy: "round-robin",
		Targets: []config.TargetConfig{
			{URL: "http://127.0.0.1:9001", Weight: 1, Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("CreateUpstreamGroup returned error: %v", err)
	}
	if len(group.Targets) != 1 {
		t.Fatalf("got %d targets, want 1", len(group.Targets))
	}

	listCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	groups, err := st.ListUpstreamGroups(listCtx)
	if err != nil {
		t.Fatalf("ListUpstreamGroups returned error: %v", err)
	}
	if len(groups) != 1 || len(groups[0].Targets) != 1 {
		t.Fatalf("got groups=%d targets=%d, want groups=1 targets=1", len(groups), len(groups[0].Targets))
	}

	target := group.Targets[0]
	target.Weight = 3
	updated, err := st.UpdateUpstreamTarget(ctx, target.ID, target)
	if err != nil {
		t.Fatalf("UpdateUpstreamTarget returned error: %v", err)
	}
	if updated.Weight != 3 {
		t.Fatalf("got weight %d, want 3", updated.Weight)
	}

	updated, err = st.SetUpstreamTargetHealthStatus(ctx, target.ID, "unhealthy")
	if err != nil {
		t.Fatalf("SetUpstreamTargetHealthStatus returned error: %v", err)
	}
	if updated.HealthStatus != "unhealthy" {
		t.Fatalf("got health status %q, want unhealthy", updated.HealthStatus)
	}
	targets, err := st.ListAllUpstreamTargets(ctx)
	if err != nil {
		t.Fatalf("ListAllUpstreamTargets returned error: %v", err)
	}
	if len(targets) != 1 || targets[0].HealthStatus != "unhealthy" {
		t.Fatalf("unexpected targets: %+v", targets)
	}

	updated, err = st.RecordUpstreamTargetHealthCheck(ctx, target.ID, "unhealthy", true, 2)
	if err != nil {
		t.Fatalf("RecordUpstreamTargetHealthCheck returned error: %v", err)
	}
	if updated.ConsecutiveFailures != 1 || !updated.Enabled {
		t.Fatalf("after first failure got enabled=%v failures=%d, want enabled=true failures=1", updated.Enabled, updated.ConsecutiveFailures)
	}

	updated, err = st.RecordUpstreamTargetHealthCheck(ctx, target.ID, "unhealthy", true, 2)
	if err != nil {
		t.Fatalf("RecordUpstreamTargetHealthCheck returned error: %v", err)
	}
	if updated.Enabled || updated.ConsecutiveFailures != 2 {
		t.Fatalf("after threshold got enabled=%v failures=%d, want enabled=false failures=2", updated.Enabled, updated.ConsecutiveFailures)
	}

	updated.Enabled = true
	updated, err = st.UpdateUpstreamTarget(ctx, target.ID, updated)
	if err != nil {
		t.Fatalf("UpdateUpstreamTarget returned error: %v", err)
	}
	if updated.ConsecutiveFailures != 2 {
		t.Fatalf("UpdateUpstreamTarget changed failures to %d, want 2", updated.ConsecutiveFailures)
	}
	updated, err = st.RecordUpstreamTargetHealthCheck(ctx, target.ID, "healthy", false, 2)
	if err != nil {
		t.Fatalf("RecordUpstreamTargetHealthCheck returned error: %v", err)
	}
	if !updated.Enabled || updated.ConsecutiveFailures != 0 || updated.HealthStatus != "healthy" {
		t.Fatalf("after recovery got enabled=%v failures=%d status=%q, want enabled=true failures=0 status=healthy", updated.Enabled, updated.ConsecutiveFailures, updated.HealthStatus)
	}

	if err := st.DeleteUpstreamTarget(ctx, target.ID); err != nil {
		t.Fatalf("DeleteUpstreamTarget returned error: %v", err)
	}
	if err := st.DeleteUpstreamGroup(ctx, group.ID); err != nil {
		t.Fatalf("DeleteUpstreamGroup returned error: %v", err)
	}
}

func TestRequestLogsCRUD(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	created, err := st.CreateRequestLog(ctx, RequestLog{
		Method:      "GET",
		Path:        "/api/users",
		RouteID:     "api-route",
		UpstreamURL: "http://127.0.0.1:9001",
		StatusCode:  200,
		DurationMS:  12,
		ClientIP:    "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("CreateRequestLog returned error: %v", err)
	}
	if created.ID == "" || created.RequestID == "" || created.CreatedAt == "" {
		t.Fatalf("expected generated ids and timestamp, got %+v", created)
	}

	logs, err := st.ListRequestLogs(ctx, 50)
	if err != nil {
		t.Fatalf("ListRequestLogs returned error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("got %d logs, want 1", len(logs))
	}
	if logs[0].Path != "/api/users" || logs[0].RouteID != "api-route" || logs[0].StatusCode != 200 {
		t.Fatalf("unexpected log: %+v", logs[0])
	}
}

func TestAuditLogsCRUD(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	created, err := st.CreateAuditLog(ctx, AuditLog{
		AdminUserID:  "usr_test",
		Action:       "create",
		ResourceType: "route",
		ResourceID:   "rte_test",
		Detail:       map[string]any{"name": "api-route"},
	})
	if err != nil {
		t.Fatalf("CreateAuditLog returned error: %v", err)
	}
	if created.ID == "" || created.CreatedAt == "" {
		t.Fatalf("expected generated id and timestamp, got %+v", created)
	}

	logs, err := st.ListAuditLogs(ctx, 20)
	if err != nil {
		t.Fatalf("ListAuditLogs returned error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("got %d logs, want 1", len(logs))
	}
	if logs[0].Action != "create" || logs[0].ResourceType != "route" || logs[0].Detail["name"] != "api-route" {
		t.Fatalf("unexpected audit log: %+v", logs[0])
	}
}
