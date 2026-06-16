package loadbalance

import (
	"testing"

	"light-api-gateway/internal/config"
)

func TestRoundRobinPicker(t *testing.T) {
	picker, err := NewPicker("round-robin", []config.TargetConfig{
		{URL: "http://a.example", Weight: 1, Enabled: true},
		{URL: "http://b.example", Weight: 1, Enabled: true},
	})
	if err != nil {
		t.Fatalf("NewPicker returned error: %v", err)
	}

	for _, want := range []string{"http://a.example", "http://b.example", "http://a.example"} {
		got, ok := picker.Next()
		if !ok {
			t.Fatal("expected target")
		}
		if got.URL != want {
			t.Fatalf("got %q, want %q", got.URL, want)
		}
	}
}

func TestWeightedRoundRobinPicker(t *testing.T) {
	picker, err := NewPicker("weighted-round-robin", []config.TargetConfig{
		{URL: "http://a.example", Weight: 2, Enabled: true},
		{URL: "http://b.example", Weight: 1, Enabled: true},
	})
	if err != nil {
		t.Fatalf("NewPicker returned error: %v", err)
	}

	for _, want := range []string{"http://a.example", "http://b.example", "http://a.example", "http://a.example"} {
		got, ok := picker.Next()
		if !ok {
			t.Fatal("expected target")
		}
		if got.URL != want {
			t.Fatalf("got %q, want %q", got.URL, want)
		}
	}
}

func TestPickerSkipsUnhealthyTargets(t *testing.T) {
	picker, err := NewPicker("round-robin", []config.TargetConfig{
		{URL: "http://healthy.example", Weight: 1, Enabled: true, HealthStatus: "healthy"},
		{URL: "http://unhealthy.example", Weight: 1, Enabled: true, HealthStatus: "unhealthy"},
		{URL: "http://unknown.example", Weight: 1, Enabled: true, HealthStatus: "unknown"},
	})
	if err != nil {
		t.Fatalf("NewPicker returned error: %v", err)
	}

	for _, want := range []string{"http://healthy.example", "http://unknown.example", "http://healthy.example"} {
		got, ok := picker.Next()
		if !ok {
			t.Fatal("expected target")
		}
		if got.URL != want {
			t.Fatalf("got %q, want %q", got.URL, want)
		}
	}
}
