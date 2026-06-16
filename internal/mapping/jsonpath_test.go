package mapping

import "testing"

func TestGet(t *testing.T) {
	source := map[string]any{
		"result": map[string]any{
			"users": []any{
				map[string]any{"name": "Tom"},
			},
		},
	}

	got, ok, err := Get(source, "$.result.users[0].name")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected path to exist")
	}
	if got != "Tom" {
		t.Fatalf("got %v, want Tom", got)
	}
}

func TestSet(t *testing.T) {
	got, err := Set(nil, "$.data.users[0].name", "Tom")
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	value, ok, err := Get(got, "$.data.users[0].name")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !ok || value != "Tom" {
		t.Fatalf("got value %v, ok %v", value, ok)
	}
}
