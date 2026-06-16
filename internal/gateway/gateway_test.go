package gateway

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"light-api-gateway/internal/config"
	"light-api-gateway/internal/mapping"
)

func TestGatewayRewritesRequestBodyAndMapsResponse(t *testing.T) {
	var upstreamPath string
	var upstreamBody map[string]any

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		if err := json.Unmarshal(body, &upstreamBody); err != nil {
			t.Fatalf("Unmarshal request body returned error: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":{"username":"Tom","userId":2}}`))
	}))
	defer upstream.Close()

	handler, err := New(config.Config{
		Gateway: config.GatewayConfig{Listen: ":0", LogRequests: false},
		Routes: []config.RouteConfig{
			{
				Name:     "api",
				Enabled:  true,
				Priority: 100,
				Type:     "proxy",
				Match:    config.MatchConfig{Path: "/api/**", Methods: []string{"POST"}},
				UpstreamGroup: config.UpstreamGroup{
					Strategy: "round-robin",
					Targets: []config.TargetConfig{
						{URL: upstream.URL, Weight: 1, Enabled: true},
					},
				},
				RequestRewrite: []config.RewriteRule{
					{Type: "rewritePath", From: "/api", To: "/v1"},
					{Type: "setJsonBody", Key: "$.meta.source", Value: "gateway"},
				},
				ResponseMapping: []config.MappingRule{
					{From: "$.result.username", To: "$.data.name"},
					{From: "$.result.userId", To: "$.data.id"},
					{Value: true, To: "$.success"},
				},
			},
		},
	}, slog.Default())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Post(server.URL+"/api/users", "application/json", strings.NewReader(`{"user":{"id":1}}`))
	if err != nil {
		t.Fatalf("Post returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want 200", resp.StatusCode)
	}
	if upstreamPath != "/v1/users" {
		t.Fatalf("upstream path got %q, want /v1/users", upstreamPath)
	}
	source, ok, err := mapping.Get(upstreamBody, "$.meta.source")
	if err != nil {
		t.Fatalf("mapping.Get returned error: %v", err)
	}
	if !ok || source != "gateway" {
		t.Fatalf("got source %v, ok %v", source, ok)
	}

	var mapped map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&mapped); err != nil {
		t.Fatalf("Decode response returned error: %v", err)
	}
	name, ok, err := mapping.Get(mapped, "$.data.name")
	if err != nil {
		t.Fatalf("mapping.Get response returned error: %v", err)
	}
	if !ok || name != "Tom" {
		t.Fatalf("got mapped name %v, ok %v", name, ok)
	}
}
