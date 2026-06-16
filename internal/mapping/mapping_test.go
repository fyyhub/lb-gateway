package mapping

import (
	"testing"

	"light-api-gateway/internal/config"
)

func TestApply(t *testing.T) {
	source := map[string]any{
		"result": map[string]any{
			"username": "Tom",
			"userId":   float64(2),
		},
	}

	got, err := Apply(source, []config.MappingRule{
		{From: "$.result.username", To: "$.data.name"},
		{From: "$.result.userId", To: "$.data.id"},
		{Value: true, To: "$.success"},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	success, ok, err := Get(got, "$.success")
	if err != nil {
		t.Fatalf("Get success returned error: %v", err)
	}
	if !ok || success != true {
		t.Fatalf("got success %v, ok %v", success, ok)
	}

	name, ok, err := Get(got, "$.data.name")
	if err != nil {
		t.Fatalf("Get name returned error: %v", err)
	}
	if !ok || name != "Tom" {
		t.Fatalf("got name %v, ok %v", name, ok)
	}
}
