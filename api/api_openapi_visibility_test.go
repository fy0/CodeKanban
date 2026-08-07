package api

import "testing"

func TestAddSchemaVisibilityToOpenAPI(t *testing.T) {
	spec := map[string]any{
		"components": map[string]any{
			"schemas": map[string]any{
				"Foo": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"$schema": map[string]any{"type": "string"},
						"name":    map[string]any{"type": "string"},
						"nested": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"bar": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}

	augmentedAny, err := addSchemaVisibilityToOpenAPI(spec)
	if err != nil {
		t.Fatalf("addSchemaVisibilityToOpenAPI returned error: %v", err)
	}

	augmented, ok := augmentedAny.(map[string]any)
	if !ok {
		t.Fatalf("expected augmented spec to be a map, got %T", augmentedAny)
	}

	components := augmented["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	foo := schemas["Foo"].(map[string]any)
	props := foo["properties"].(map[string]any)

	if got := props["$schema"].(map[string]any)["visible"]; got != false {
		t.Fatalf("expected $schema.visible=false, got %#v", got)
	}
	if got := props["name"].(map[string]any)["visible"]; got != true {
		t.Fatalf("expected name.visible=true, got %#v", got)
	}

	nested := props["nested"].(map[string]any)
	nestedProps := nested["properties"].(map[string]any)
	if got := nestedProps["bar"].(map[string]any)["visible"]; got != true {
		t.Fatalf("expected nested.bar.visible=true, got %#v", got)
	}
}
