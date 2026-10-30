package deploy

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

const exampleDocument = `
spec: deploy/v1
environments:
  production:
    branch: main
    cluster: us-east-1
    interfaces: [web, slack]
    public: { web: false, custom: true }
    variables:
      LOG_LEVEL: info
      PORT: 8080
      OPENAI_API_KEY: { ref: OPENAI_API_KEY }
    models: { default: claude-sonnet-5-5 }
    schedules: { nightly-sync: "0 3 * * *" }
    knowledge: { docs: kb_docs_prod }
    resources:
      agent: { cpu: "1", memory: 2Gi }
      ingestion: { nightly-sync: { memory: 1Gi } }
  staging:
    branch: develop
`

func TestParseMapsEachEnvironmentOntoATemplateRequest(t *testing.T) {
	doc, err := Parse(exampleDocument)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if got, want := doc.Names(), []string{"production", "staging"}; !slices.Equal(got, want) {
		t.Fatalf("Names() = %v, want %v in sorted order", got, want)
	}

	cases := map[string]string{
		"production": `{"cluster_id":"us-east-1","interfaces":{"adapters":["web","slack"],"auth":{"web":{"public":false},"custom":{"public":true}}},"variables":{"LOG_LEVEL":{"value":"info"},"OPENAI_API_KEY":{"ref":"OPENAI_API_KEY"},"PORT":{"value":"8080"}},"models":{"default":"claude-sonnet-5-5"},"schedules":{"nightly-sync":"0 3 * * *"},"bindings":{"knowledge":{"docs":"kb_docs_prod"}},"provisioning":{"agent":{"compute":{"cpu":"1","memory":"2Gi"}},"ingestion":{"nightly-sync":{"compute":{"memory":"1Gi"}}}}}`,
		"staging":    `{}`,
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := json.Marshal(doc.Environments[name].TemplateRequest())
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if string(got) != want {
				t.Errorf("TemplateRequest() = %s\nwant %s\n(a field the file leaves out must stay out, so the server keeps its stored value)", got, want)
			}
		})
	}
	if got := doc.Environments["staging"].Branch; got != "develop" {
		t.Errorf("staging Branch = %q, want develop", got)
	}
}

func TestParseRejectsInvalidDocuments(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"wrong spec", "spec: deploy/v2\nenvironments: {a: {}}\n", `spec must be "deploy/v1"`},
		{"no environments", "spec: deploy/v1\nenvironments: {}\n", "at least one environment"},
		{"unknown key", "spec: deploy/v1\nenvironments: {a: {blueprint: x}}\n", "field blueprint not found"},
		{"unknown interface", "spec: deploy/v1\nenvironments: {a: {interfaces: [email]}}\n", `unknown interface "email"`},
		{"public without interfaces", "spec: deploy/v1\nenvironments: {a: {public: {web: true}}}\n", "public needs interfaces"},
		{"variable mapping other than ref", "spec: deploy/v1\nenvironments: {a: {variables: {X: {value: y}}}}\n", "{ ref: NAME }"},
		{"empty ref", "spec: deploy/v1\nenvironments: {a: {variables: {X: {ref: \"\"}}}}\n", "ref must name a variable"},
		{"empty variable", "spec: deploy/v1\nenvironments: {a: {variables: {X: \"\"}}}\n", "X has no value"},
		{"empty compute", "spec: deploy/v1\nenvironments: {a: {resources: {agent: {}}}}\n", "agent sets neither cpu nor memory"},
		{"environment name with spaces", "spec: deploy/v1\nenvironments: {\" a\": {}}\n", "no surrounding spaces"},
		{"two documents", "spec: deploy/v1\nenvironments: {a: {}}\n---\nspec: deploy/v1\n", "exactly one YAML document"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.yaml)
			if !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("Parse() error = %v, want ErrInvalidDocument", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Parse() error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestParseRejectsAnOversizedDocument(t *testing.T) {
	_, err := Parse("spec: deploy/v1\n" + strings.Repeat("#", maxDocumentBytes))
	if !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("Parse() error = %v, want ErrInvalidDocument for a document over %d bytes", err, maxDocumentBytes)
	}
}
