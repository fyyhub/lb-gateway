package router

import (
	"net/http"
	"testing"

	"light-api-gateway/internal/config"
)

func TestRouterMatchesHighestPriorityRoute(t *testing.T) {
	rt, skipped := New(config.Config{
		Routes: []config.RouteConfig{
			{
				Name:     "wildcard",
				Enabled:  true,
				Priority: 10,
				Type:     "redirect",
				Match:    config.MatchConfig{Path: "/api/**", Methods: []string{"GET"}},
				Redirect: &config.RedirectConfig{
					Targets: []config.TargetConfig{{URL: "http://wildcard.example", Enabled: true}},
				},
			},
			{
				Name:     "specific",
				Enabled:  true,
				Priority: 20,
				Type:     "redirect",
				Match:    config.MatchConfig{Path: "/api/users", Methods: []string{"GET"}},
				Redirect: &config.RedirectConfig{
					Targets: []config.TargetConfig{{URL: "http://specific.example", Enabled: true}},
				},
			},
		},
	})
	if len(skipped) != 0 {
		t.Fatalf("New skipped routes: %v", skipped)
	}

	req, err := http.NewRequest(http.MethodGet, "http://localhost/api/users", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	route, ok := rt.Match(req)
	if !ok {
		t.Fatal("expected route")
	}
	if route.Config.Name != "specific" {
		t.Fatalf("got route %q, want %q", route.Config.Name, "specific")
	}
}

func TestRouterDoesNotMatchDisabledRoute(t *testing.T) {
	rt, skipped := New(config.Config{
		Routes: []config.RouteConfig{
			{
				Name:     "disabled",
				Enabled:  false,
				Priority: 10,
				Type:     "redirect",
				Match:    config.MatchConfig{Path: "/web"},
				Redirect: &config.RedirectConfig{
					Targets: []config.TargetConfig{{URL: "http://disabled.example", Enabled: true}},
				},
			},
		},
	})
	if len(skipped) != 0 {
		t.Fatalf("New skipped routes: %v", skipped)
	}

	req, err := http.NewRequest(http.MethodGet, "http://localhost/web", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	if _, ok := rt.Match(req); ok {
		t.Fatal("did not expect route")
	}
}

func TestRouterSkipsInvalidRoutesAndKeepsValidOnes(t *testing.T) {
	rt, skipped := New(config.Config{
		Routes: []config.RouteConfig{
			{
				Name:     "broken-proxy",
				Enabled:  true,
				Priority: 50,
				Type:     "proxy",
				Match:    config.MatchConfig{Path: "/api/**"},
				UpstreamGroup: config.UpstreamGroup{
					Strategy: "round-robin",
					Targets:  []config.TargetConfig{}, // no usable targets -> picker fails
				},
			},
			{
				Name:     "healthy-redirect",
				Enabled:  true,
				Priority: 10,
				Type:     "redirect",
				Match:    config.MatchConfig{Path: "/web"},
				Redirect: &config.RedirectConfig{
					Targets: []config.TargetConfig{{URL: "http://ok.example", Enabled: true}},
				},
			},
		},
	})

	if len(skipped) != 1 {
		t.Fatalf("got %d skipped routes, want 1: %v", len(skipped), skipped)
	}
	if skipped[0].Name != "broken-proxy" {
		t.Fatalf("skipped route %q, want %q", skipped[0].Name, "broken-proxy")
	}

	webReq, err := http.NewRequest(http.MethodGet, "http://localhost/web", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	route, ok := rt.Match(webReq)
	if !ok {
		t.Fatal("expected the valid redirect route to remain active")
	}
	if route.Config.Name != "healthy-redirect" {
		t.Fatalf("matched route %q, want %q", route.Config.Name, "healthy-redirect")
	}

	apiReq, err := http.NewRequest(http.MethodGet, "http://localhost/api/users", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	if _, ok := rt.Match(apiReq); ok {
		t.Fatal("did not expect the skipped proxy route to match")
	}
}
