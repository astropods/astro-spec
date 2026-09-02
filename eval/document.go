package eval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	schemaV1 = "evaluation/v1"

	minEvaluators = 1
	maxEvaluators = 10

	maxTotalBytes = 128 * 1024
)

var (
	ErrInvalidDocument = errors.New("invalid evaluation document")

	// ErrUnknownRef marks a "ref" value that names no known preset evaluator.
	ErrUnknownRef = errors.New("unknown preset reference")
)

type Entry struct {
	Ref         string  `json:"ref,omitempty"`
	Key         string  `json:"key,omitempty"`
	Label       string  `json:"label,omitempty"`
	Description string  `json:"description,omitempty"`
	Type        string  `json:"type,omitempty"`
	Config      *Config `json:"config,omitempty"`
	Prompt      string  `json:"prompt,omitempty"`
	Output      *Output `json:"output,omitempty"`
}

type Document struct {
	Schema     string  `json:"schema"`
	Evaluators []Entry `json:"evaluators"`
}

type Result struct {
	EvaluationRef  string
	DefinitionJSON json.RawMessage
	Document       Document
}

type rawDocument struct {
	Schema     string     `yaml:"schema"`
	Evaluators []rawEntry `yaml:"evaluators"`
}

type rawEntry struct {
	Ref         string     `yaml:"ref"`
	Key         string     `yaml:"key"`
	Label       string     `yaml:"label"`
	Description string     `yaml:"description"`
	Type        string     `yaml:"type"`
	Config      *rawConfig `yaml:"config"`
	Prompt      string     `yaml:"prompt"`
	PromptFile  string     `yaml:"prompt_file"`
	Output      *rawOutput `yaml:"output"`
}

type rawConfig struct {
	Context *rawContext `yaml:"context"`
}

type rawContext struct {
	PreviousTurns   *bool    `yaml:"previous_turns"`
	NextUserMessage *bool    `yaml:"next_user_message"`
	UserFeedback    *bool    `yaml:"user_feedback"`
	Steps           *bool    `yaml:"steps"`
	StepTypes       []string `yaml:"step_types"`
}

type rawOutput struct {
	Type      string   `yaml:"type"`
	Options   []string `yaml:"options"`
	Minimum   *float64 `yaml:"minimum"`
	Maximum   *float64 `yaml:"maximum"`
	MaxLength *int     `yaml:"max_length"`
}

// Parse validates an EVALUATION.yaml document and its referenced prompt
// files, entirely offline: it never resolves a preset ref to its prompt
// text, only confirms the ref names a known preset.
func Parse(yamlText string, promptFiles map[string]string) (Result, error) {
	if size := totalBytes(yamlText, promptFiles); size > maxTotalBytes {
		return Result{}, invalidDocument("document and prompt files total %d bytes, exceeding %d", size, maxTotalBytes)
	}

	raw, err := decodeStrict(yamlText)
	if err != nil {
		return Result{}, err
	}

	if raw.Schema != schemaV1 {
		return Result{}, invalidDocument("schema must be %q, got %q", schemaV1, raw.Schema)
	}
	if len(raw.Evaluators) < minEvaluators || len(raw.Evaluators) > maxEvaluators {
		return Result{}, invalidDocument("document must contain %d to %d evaluators, got %d",
			minEvaluators, maxEvaluators, len(raw.Evaluators))
	}

	usedPromptFiles := make(map[string]bool, len(promptFiles))
	seenKeys := make(map[string]bool, len(raw.Evaluators))
	entries := make([]Entry, 0, len(raw.Evaluators))

	for i, r := range raw.Evaluators {
		entry, key, err := normalizeEntry(r, promptFiles, usedPromptFiles)
		if err != nil {
			return Result{}, invalidDocument("evaluator %d: %v", i, err)
		}
		if seenKeys[key] {
			return Result{}, invalidDocument("duplicate evaluator key %q", key)
		}
		seenKeys[key] = true
		entries = append(entries, entry)
	}

	for filePath := range promptFiles {
		if !usedPromptFiles[filePath] {
			return Result{}, invalidDocument("prompt file %q is not referenced by any evaluator", filePath)
		}
	}

	doc := Document{Schema: raw.Schema, Evaluators: entries}
	ref, canonical, err := evaluationRef(doc)
	if err != nil {
		return Result{}, fmt.Errorf("eval compute ref: %w", err)
	}

	return Result{EvaluationRef: ref, DefinitionJSON: json.RawMessage(canonical), Document: doc}, nil
}

func decodeStrict(yamlText string) (rawDocument, error) {
	var raw rawDocument
	dec := yaml.NewDecoder(strings.NewReader(yamlText))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		return rawDocument{}, invalidDocument("%v", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return rawDocument{}, invalidDocument("document must contain exactly one YAML document")
	}
	return raw, nil
}

// normalizeEntry validates one raw evaluator entry and returns its
// normalized Entry plus the evaluator key it resolves to, for duplicate
// detection across both preset and custom entries.
func normalizeEntry(
	r rawEntry,
	promptFiles map[string]string,
	usedPromptFiles map[string]bool,
) (Entry, string, error) {
	if strings.TrimSpace(r.Ref) != "" {
		return normalizeRefEntry(r)
	}
	return normalizeCustomEntry(r, promptFiles, usedPromptFiles)
}

func normalizeRefEntry(r rawEntry) (Entry, string, error) {
	if r.Key != "" || r.Label != "" || r.Description != "" || r.Type != "" || r.Config != nil || r.Prompt != "" || r.PromptFile != "" || r.Output != nil {
		return Entry{}, "", fmt.Errorf("a preset reference accepts no other fields")
	}
	if !IsPresetRef(r.Ref) {
		return Entry{}, "", fmt.Errorf("%w %q, available presets: %s", ErrUnknownRef, r.Ref, strings.Join(PresetRefs(), ", "))
	}
	return Entry{Ref: r.Ref}, presetKeyFor(r.Ref), nil
}

func normalizeCustomEntry(
	r rawEntry,
	promptFiles map[string]string,
	usedPromptFiles map[string]bool,
) (Entry, string, error) {
	if r.Key == "" && r.Label == "" && r.Type == "" && r.Prompt == "" && r.PromptFile == "" && r.Output == nil {
		return Entry{}, "", fmt.Errorf("must be a preset reference or a complete custom definition")
	}

	prompt, err := resolvePrompt(r, promptFiles, usedPromptFiles)
	if err != nil {
		return Entry{}, "", err
	}

	config := resolveConfig(r.Config)
	output := resolveOutput(r.Output)

	def := Evaluator{
		Key:         r.Key,
		Label:       r.Label,
		Description: r.Description,
		Type:        Type(r.Type),
		Config:      config,
		Prompt:      prompt,
		Output:      output,
	}
	if err := Validate(def); err != nil {
		return Entry{}, "", err
	}

	entry := Entry{
		Key:         def.Key,
		Label:       def.Label,
		Description: def.Description,
		Type:        string(def.Type),
		Config:      &config,
		Prompt:      def.Prompt,
		Output:      &output,
	}
	return entry, def.Key, nil
}

func resolvePrompt(r rawEntry, promptFiles map[string]string, usedPromptFiles map[string]bool) (string, error) {
	hasPrompt := strings.TrimSpace(r.Prompt) != ""
	hasPromptFile := strings.TrimSpace(r.PromptFile) != ""
	if hasPrompt && hasPromptFile {
		return "", fmt.Errorf("must set exactly one of prompt or prompt_file")
	}
	if hasPrompt {
		return normalizeLineEndings(r.Prompt), nil
	}
	if !hasPromptFile {
		return "", fmt.Errorf("must set exactly one of prompt or prompt_file")
	}
	if !validPromptFilePath(r.PromptFile) {
		return "", fmt.Errorf("prompt_file %q must be a relative .md path within the project", r.PromptFile)
	}
	contents, ok := promptFiles[r.PromptFile]
	if !ok {
		return "", fmt.Errorf("prompt file %q was not provided", r.PromptFile)
	}
	if !utf8.ValidString(contents) {
		return "", fmt.Errorf("prompt file %q is not valid UTF-8", r.PromptFile)
	}
	usedPromptFiles[r.PromptFile] = true
	return normalizeLineEndings(contents), nil
}

func validPromptFilePath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || !strings.HasSuffix(p, ".md") {
		return false
	}
	cleaned := path.Clean(p)
	return cleaned == p && cleaned != ".." && !strings.HasPrefix(cleaned, "../")
}

func normalizeLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func resolveConfig(c *rawConfig) Config {
	if c == nil || c.Context == nil {
		return Config{}
	}
	ctx := c.Context
	config := Config{}
	if ctx.PreviousTurns != nil {
		config.Context.PreviousTurns = *ctx.PreviousTurns
	}
	if ctx.NextUserMessage != nil {
		config.Context.NextUserMessage = *ctx.NextUserMessage
	}
	if ctx.UserFeedback != nil {
		config.Context.UserFeedback = *ctx.UserFeedback
	}
	if ctx.Steps != nil {
		config.Context.Steps = *ctx.Steps
	}
	config.Context.StepTypes = ctx.StepTypes
	return config
}

func resolveOutput(o *rawOutput) Output {
	if o == nil {
		return Output{}
	}
	return Output{
		Type:      OutputType(o.Type),
		Options:   o.Options,
		Minimum:   o.Minimum,
		Maximum:   o.Maximum,
		MaxLength: o.MaxLength,
	}
}

func totalBytes(yamlText string, promptFiles map[string]string) int {
	total := len(yamlText)
	for _, contents := range promptFiles {
		total += len(contents)
	}
	return total
}

func invalidDocument(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidDocument, fmt.Sprintf(format, args...))
}
