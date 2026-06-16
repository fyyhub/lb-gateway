package rewrite

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"light-api-gateway/internal/config"
)

func TestApplyRewriteRules(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://localhost/api/users?page=1", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	err = Apply(req, []config.RewriteRule{
		{Type: "setHeader", Key: "X-Gateway", Value: "light-api-gateway"},
		{Type: "setQuery", Key: "source", Value: "gateway"},
		{Type: "rewritePath", From: "/api", To: "/v1"},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if got := req.Header.Get("X-Gateway"); got != "light-api-gateway" {
		t.Fatalf("header got %q", got)
	}
	if got := req.URL.Query().Get("source"); got != "gateway" {
		t.Fatalf("query got %q", got)
	}
	if req.URL.Path != "/v1/users" {
		t.Fatalf("path got %q", req.URL.Path)
	}
}

func TestApplySetJSONBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://localhost/api/users", strings.NewReader(`{"user":{"id":1}}`))
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	err = Apply(req, []config.RewriteRule{
		{Type: "setJsonBody", Key: "$.user.name", Value: "Tom"},
		{Type: "setJsonBody", Key: "$.meta.source", Value: "gateway"},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	user := decoded["user"].(map[string]any)
	if user["name"] != "Tom" {
		t.Fatalf("got user.name %v", user["name"])
	}
	meta := decoded["meta"].(map[string]any)
	if meta["source"] != "gateway" {
		t.Fatalf("got meta.source %v", meta["source"])
	}
}
