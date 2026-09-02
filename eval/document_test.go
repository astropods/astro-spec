package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestParseAcceptsAPresetOnlyDocument(t *testing.T) {
	result, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
  - ref: preset/user-sentiment
`, nil)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if len(result.Document.Evaluators) != 2 {
		t.Fatalf("len(Document.Evaluators) = %d, want 2", len(result.Document.Evaluators))
	}
	if got := result.Document.Evaluators[0].Ref; got != "preset/exposed-pii" {
		t.Errorf("Evaluators[0].Ref = %q, want %q", got, "preset/exposed-pii")
	}
	if got := result.Document.Evaluators[1].Ref; got != "preset/user-sentiment" {
		t.Errorf("Evaluators[1].Ref = %q, want %q", got, "preset/user-sentiment")
	}
	if !strings.HasPrefix(result.EvaluationRef, "agent/") {
		t.Errorf("EvaluationRef = %q, want agent/ prefix", result.EvaluationRef)
	}
}

func TestParseAcceptsACustomEvaluator(t *testing.T) {
	result, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    description: Flags credentials in the output.
    type: llm
    prompt: Determine whether the agent output exposes credentials.
    output:
      type: boolean
`, nil)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if len(result.Document.Evaluators) != 1 {
		t.Fatalf("len(Document.Evaluators) = %d, want 1", len(result.Document.Evaluators))
	}
	entry := result.Document.Evaluators[0]
	if entry.Key != "has_secrets" {
		t.Errorf("Key = %q, want %q", entry.Key, "has_secrets")
	}
	if entry.Description != "Flags credentials in the output." {
		t.Errorf("Description = %q, want %q", entry.Description, "Flags credentials in the output.")
	}
	if entry.Output == nil || entry.Output.Type != OutputBoolean {
		t.Errorf("Output = %+v, want Type %q", entry.Output, OutputBoolean)
	}
}

func TestParseInlinesAPromptFile(t *testing.T) {
	result, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: response_quality
    label: Response quality
    type: llm
    prompt_file: evaluation/response-quality.md
    output:
      type: number
      minimum: 0
      maximum: 1
`, map[string]string{
		"evaluation/response-quality.md": "Assess the overall quality of the response.",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if len(result.Document.Evaluators) != 1 {
		t.Fatalf("len(Document.Evaluators) = %d, want 1", len(result.Document.Evaluators))
	}
	if got := result.Document.Evaluators[0].Prompt; got != "Assess the overall quality of the response." {
		t.Errorf("Prompt = %q, want %q", got, "Assess the overall quality of the response.")
	}
}

func TestParseAllowsStepsContextOnACustomEvaluator(t *testing.T) {
	result, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: redundant_call
    label: Redundant tool call
    type: llm
    config:
      context:
        steps: true
        step_types:
          - tool
    prompt: Determine whether a tool call was redundant.
    output:
      type: boolean
`, nil)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if len(result.Document.Evaluators) != 1 {
		t.Fatalf("len(Document.Evaluators) = %d, want 1", len(result.Document.Evaluators))
	}
	config := result.Document.Evaluators[0].Config
	if config == nil || !config.Context.Steps {
		t.Fatalf("Config = %+v, want Context.Steps = true", config)
	}
	if len(config.Context.StepTypes) != 1 || config.Context.StepTypes[0] != "tool" {
		t.Errorf("Context.StepTypes = %v, want [tool]", config.Context.StepTypes)
	}
}

func TestParseRejectsStepTypesWithoutSteps(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: redundant_call
    label: Redundant tool call
    type: llm
    config:
      context:
        step_types:
          - tool
    prompt: Determine whether a tool call was redundant.
    output:
      type: boolean
`, nil)
	requireInvalidDocument(t, err)
}

func TestParseRejectsWrongSchema(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v2
evaluators:
  - ref: preset/exposed-pii
`, nil)
	requireInvalidDocument(t, err)
}

func TestParseRejectsAMultiDocumentYAMLStream(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
---
schema: evaluation/v1
evaluators:
  - ref: preset/leaked-credentials
`, nil)
	requireInvalidDocument(t, err)
}

func TestParseRejectsTooFewEvaluators(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators: []
`, nil)
	requireInvalidDocument(t, err)
}

func TestParseRejectsTooManyEvaluators(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("schema: evaluation/v1\nevaluators:\n")
	for i := 0; i < 11; i++ {
		sb.WriteString("  - ref: preset/exposed-pii\n")
	}
	_, err := Parse(sb.String(), nil)
	requireInvalidDocument(t, err)
}

func TestParseRejectsDuplicateKeysAfterPresetResolution(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
  - key: exposed_pii
    label: Duplicate
    type: llm
    prompt: Some other prompt for this evaluator.
    output:
      type: boolean
`, nil)
	requireInvalidDocument(t, err)
}

func TestParseRejectsAPresetRefWithExtraFields(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
    label: Overridden label
`, nil)
	requireInvalidDocument(t, err)
}

func TestParseRejectsAnUnknownPresetRef(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/does-not-exist
`, nil)
	requireInvalidDocument(t, err)
	if !strings.Contains(err.Error(), "available presets: preset/exposed-pii") {
		t.Errorf("error = %q, want it to list available presets", err.Error())
	}
}

func TestParseRejectsAnUnknownTopLevelField(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
extra: not allowed
evaluators:
  - ref: preset/exposed-pii
`, nil)
	requireInvalidDocument(t, err)
}

func TestParseRejectsAnUnknownEvaluatorField(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt: Determine whether the agent output exposes credentials.
    output:
      type: boolean
    extra: not allowed
`, nil)
	requireInvalidDocument(t, err)
}

func TestParseRejectsBothPromptAndPromptFile(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt: Inline prompt.
    prompt_file: evaluation/has-secrets.md
    output:
      type: boolean
`, map[string]string{"evaluation/has-secrets.md": "File prompt."})
	requireInvalidDocument(t, err)
}

func TestParseRejectsAMissingPromptFile(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt_file: evaluation/has-secrets.md
    output:
      type: boolean
`, nil)
	requireInvalidDocument(t, err)
}

func TestParseRejectsAnUnreferencedPromptFile(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
`, map[string]string{"evaluation/unused.md": "Not referenced by anything."})
	requireInvalidDocument(t, err)
}

func TestParseRejectsANonMarkdownPromptFile(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt_file: evaluation/has-secrets.txt
    output:
      type: boolean
`, map[string]string{"evaluation/has-secrets.txt": "File prompt."})
	requireInvalidDocument(t, err)
}

func TestParseRejectsAPathTraversalPromptFile(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt_file: ../outside/has-secrets.md
    output:
      type: boolean
`, map[string]string{"../outside/has-secrets.md": "File prompt."})
	requireInvalidDocument(t, err)
}

func TestParseRejectsDocumentsOverTheSizeCap(t *testing.T) {
	huge := strings.Repeat("a", maxTotalBytes+1)
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt_file: evaluation/has-secrets.md
    output:
      type: boolean
`, map[string]string{"evaluation/has-secrets.md": huge})
	requireInvalidDocument(t, err)
}

func TestDefinitionJSONIsWhatTheEvaluationRefHashes(t *testing.T) {
	result, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
`, nil)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	sum := sha256.Sum256(result.DefinitionJSON)
	want := evaluationRefPrefix + hex.EncodeToString(sum[:])
	if result.EvaluationRef != want {
		t.Errorf("EvaluationRef = %q, want %q", result.EvaluationRef, want)
	}
}

func TestEvaluationRefIsStableAcrossFormattingChanges(t *testing.T) {
	a, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
  - ref: preset/user-sentiment
`, nil)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	b, err := Parse("schema: evaluation/v1\n# a comment\nevaluators:\n  - ref: preset/exposed-pii   # trailing comment\n  - ref: preset/user-sentiment\n", nil)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	if a.EvaluationRef != b.EvaluationRef {
		t.Errorf("EvaluationRef differs across formatting: %q vs %q", a.EvaluationRef, b.EvaluationRef)
	}
}

func TestEvaluationRefChangesWithEvaluatorOrder(t *testing.T) {
	a, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
  - ref: preset/user-sentiment
`, nil)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	b, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/user-sentiment
  - ref: preset/exposed-pii
`, nil)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	if a.EvaluationRef == b.EvaluationRef {
		t.Errorf("EvaluationRef should change with evaluator order, got %q for both", a.EvaluationRef)
	}
}

func TestEvaluationRefDoesNotEmbedPresetContent(t *testing.T) {
	result, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
  - ref: preset/user-sentiment
`, nil)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if len(result.Document.Evaluators) != 2 {
		t.Fatalf("len(Document.Evaluators) = %d, want 2", len(result.Document.Evaluators))
	}
	entry := result.Document.Evaluators[0]
	if entry.Ref != "preset/exposed-pii" {
		t.Errorf("Ref = %q, want %q", entry.Ref, "preset/exposed-pii")
	}
	if entry.Key != "" {
		t.Errorf("Key = %q, want empty", entry.Key)
	}
	if entry.Config != nil {
		t.Errorf("Config = %+v, want nil", entry.Config)
	}
	if entry.Output != nil {
		t.Errorf("Output = %+v, want nil", entry.Output)
	}
}

func TestEvaluationRefChangesWithCustomContent(t *testing.T) {
	a, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt: Determine whether the agent output exposes credentials.
    output:
      type: boolean
`, nil)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	b, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt: Determine whether the agent output exposes an API key.
    output:
      type: boolean
`, nil)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	if a.EvaluationRef == b.EvaluationRef {
		t.Errorf("EvaluationRef should change with prompt content, got %q for both", a.EvaluationRef)
	}
}

func requireInvalidDocument(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("Parse() error = nil, want an error")
	}
	if !errors.Is(err, ErrInvalidDocument) {
		t.Errorf("Parse() error = %v, want ErrInvalidDocument", err)
	}
}
