package spec

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSchema_MetaFieldsDeclared pins the RFC-1 §2.1 contract (echoed by the Agent
// Card migration docs): meta.description, meta.tags, and meta.visibility are
// DEPRECATED but still ACCEPTED — description/tags are read as a fallback when no
// AGENT.md is present. Because the meta object sets additionalProperties:false,
// the fields must stay declared in the schema or valid specs get rejected. This
// guards against silently dropping them again.
func TestSchema_MetaFieldsDeclared(t *testing.T) {
	var doc struct {
		Properties struct {
			Meta struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"meta"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(Schema(), &doc); err != nil {
		t.Fatalf("unmarshal embedded schema: %v", err)
	}

	for _, field := range []string{"description", "tags", "visibility"} {
		if _, ok := doc.Properties.Meta.Properties[field]; !ok {
			t.Errorf("meta.%s missing from schema — deprecated fields must stay accepted (RFC-1 §2.1), not removed", field)
		}
	}
}

// A jsonschema struct tag is comma-delimited, so a comma inside a description
// silently truncates it at generation time. This caught a 461-character
// description being cut to 267.
func TestSchema_DevWatchDescriptionIsWhole(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(astroSchema, &doc); err != nil {
		t.Fatalf("unmarshal embedded schema: %v", err)
	}

	props := doc["properties"].(map[string]any)
	dev := props["dev"].(map[string]any)["properties"].(map[string]any)
	watch, ok := dev["watch"].(map[string]any)
	if !ok {
		t.Fatal("dev.watch missing from the generated schema")
	}

	desc, _ := watch["description"].(string)
	for _, want := range []string{
		"List every directory your start command imports",
		"Leave out any directory your Dockerfile builds",
		"does not exist is skipped",
	} {
		if !strings.Contains(desc, want) {
			t.Errorf("dev.watch description is missing %q; a comma in the tag truncates it", want)
		}
	}
}
