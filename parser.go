package spec

import (
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"

	"gopkg.in/yaml.v3"
)

// unquotedAtName matches a top-level `name:` value starting with an unquoted @.
var unquotedAtName = regexp.MustCompile(`(?m)^name:\s+@`)

var validDatatypes = map[string]bool{
	"string": true, "boolean": true, "number": true, "array": true, "object": true,
}

var validDisplayAs = map[string]bool{
	"short-text": true, "long-text": true, "select": true,
}

var validScopes = map[string]bool{"models": true, "knowledge": true, "integrations": true}

var validTriggerTypes = map[string]bool{"schedule": true, "startup": true, "manual": true, "webhook": true}

// Problem is a single validation failure. Field is the dotted YAML path the
// failure concerns, so callers can attribute it to a location in the document.
type Problem struct {
	Field   string
	Message string
}

func (p Problem) Error() string { return p.Message }

// Parse parses and validates spec content. It is the single parse
// implementation; the path- and string-based entry points delegate here.
func Parse(data []byte) (*AstroSpec, error) {
	var s AstroSpec
	if err := yaml.Unmarshal(data, &s); err != nil {
		if unquotedAtName.Match(data) {
			return nil, fmt.Errorf("failed to parse spec: the @ character in the name field must be quoted, e.g. name: \"@org/agent\"")
		}
		return nil, fmt.Errorf("failed to parse spec: %w", err)
	}
	if err := Validate(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// ParseString parses and validates the spec content from a string.
func ParseString(content string) (*AstroSpec, error) {
	return Parse([]byte(content))
}

// ParseFile reads, parses, and validates a spec file from the given path.
func ParseFile(path string) (*AstroSpec, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to read spec file: %w", err)
	}
	return Parse(data)
}

// ParseSpec reads, parses, and validates a spec file from the given path.
func ParseSpec(path string) (*AstroSpec, error) {
	return ParseFile(path)
}

// Validate reports the first validation failure in s, or nil when s is valid.
// It is a thin wrapper over Problems so the two can never disagree.
func Validate(s *AstroSpec) error {
	if problems := Problems(s); len(problems) > 0 {
		return problems[0]
	}
	return nil
}

// Problems returns every validation failure in s. Use it to report all
// failures at once; use Validate to fail on the first. Map-keyed sections are
// visited in sorted key order, so the result is deterministic.
//
// Every check here is decidable from the spec alone. Rules that need
// deploy-time input (credential values, schedule expressions, enabled
// interfaces) or server policy belong to the caller.
func Problems(s *AstroSpec) []Problem {
	var problems []Problem
	add := func(field, format string, args ...any) {
		problems = append(problems, Problem{Field: field, Message: fmt.Sprintf(format, args...)})
	}

	if s.Spec == "" {
		add("spec", "spec version is required")
	}
	if s.Name == "" {
		add("name", "agent name is required")
	} else if _, name := SplitAgentName(s.Name); ValidateName(name) != nil {
		add("name", "name %q is invalid: %v", s.Name, ValidateName(name))
	}

	if s.Agent.Build == nil && s.Agent.Image == "" {
		add("agent", "agent.build or agent.image is required")
	}
	if s.Agent.Build != nil && s.Agent.Image != "" {
		add("agent", "agent: image and build are mutually exclusive")
	}
	if s.Agent.Build != nil {
		problems = append(problems, buildProblems("agent.build", s.Agent.Build)...)
	}

	for _, key := range slices.Sorted(maps.Keys(s.Inputs)) {
		problems = append(problems, inputProblems(fmt.Sprintf("inputs.%s", key), s.Inputs[key])...)
	}
	for i, input := range s.Agent.Inputs {
		problems = append(problems, inputProblems(fmt.Sprintf("agent.inputs[%d]", i), input)...)
	}

	for _, name := range slices.Sorted(maps.Keys(s.Providers)) {
		provider := s.Providers[name]
		if len(provider.Scope) == 0 {
			add(fmt.Sprintf("providers.%s.scope", name),
				"providers.%s: scope is required and must contain at least one of: models, knowledge, integrations", name)
		}
		for _, scope := range provider.Scope {
			if !validScopes[scope] {
				add(fmt.Sprintf("providers.%s.scope", name),
					"providers.%s: invalid scope value %q (must be one of: models, knowledge, integrations)", name, scope)
			}
		}
		if len(provider.Variables) == 0 {
			add(fmt.Sprintf("providers.%s.variables", name),
				"providers.%s: variables is required and must contain at least one entry", name)
		}
		for i, v := range provider.Variables {
			problems = append(problems, inputProblems(fmt.Sprintf("providers.%s.variables[%d]", name, i), v)...)
		}
	}

	for _, name := range slices.Sorted(maps.Keys(s.Knowledge)) {
		k := s.Knowledge[name]
		if k.Provider != "" && k.Container != nil {
			add(fmt.Sprintf("knowledge.%s", name), "knowledge %q: provider and container are mutually exclusive", name)
		}
		if k.Provider == "" && k.Container == nil {
			add(fmt.Sprintf("knowledge.%s", name), "knowledge %q: either provider or container is required", name)
		}
		if k.Provider != "" {
			problems = append(problems, scopeProblems(s, "knowledge", name, k.Provider)...)
		}
		problems = append(problems, containerProblems(fmt.Sprintf("knowledge.%s", name), k.Container)...)
		for i, input := range k.Inputs {
			problems = append(problems, inputProblems(fmt.Sprintf("knowledge.%s.inputs[%d]", name, i), input)...)
		}
	}

	for _, name := range slices.Sorted(maps.Keys(s.Models)) {
		m := s.Models[name]
		if m.Provider != "" && m.Container != nil {
			add(fmt.Sprintf("models.%s", name), "model %q: provider and container are mutually exclusive", name)
		}
		if m.Provider == "" && m.Container == nil {
			add(fmt.Sprintf("models.%s", name), "model %q: either provider or container is required", name)
		}
		if len(m.Models) > 0 && m.Model != "" {
			add(fmt.Sprintf("models.%s", name), "model %q: models and model are mutually exclusive", name)
		}
		if m.Provider != "" {
			problems = append(problems, scopeProblems(s, "models", name, m.Provider)...)
		}
		problems = append(problems, containerProblems(fmt.Sprintf("models.%s", name), m.Container)...)
		for i, input := range m.Inputs {
			problems = append(problems, inputProblems(fmt.Sprintf("models.%s.inputs[%d]", name, i), input)...)
		}
	}

	// The deprecated agent.astro_ai_gateway boolean and a provider: gateway model
	// both enable the gateway; requiring exactly one keeps enablement unambiguous.
	if s.Agent.AIGateway && slices.ContainsFunc(slices.Collect(maps.Values(s.Models)), Model.IsGateway) {
		add("agent.astro_ai_gateway",
			"agent.astro_ai_gateway and a model with provider: %q are mutually exclusive; use the gateway model entry",
			GatewayProviderName)
	}

	for _, name := range slices.Sorted(maps.Keys(s.Integrations)) {
		t := s.Integrations[name]
		if t.Provider != "" && t.Container != nil {
			add(fmt.Sprintf("integrations.%s", name), "integration %q: provider and container are mutually exclusive", name)
		}
		if t.Provider == "" && t.Container == nil {
			add(fmt.Sprintf("integrations.%s", name), "integration %q: either provider or container is required", name)
		}
		if t.Provider != "" {
			problems = append(problems, scopeProblems(s, "integrations", name, t.Provider)...)
		}
		problems = append(problems, containerProblems(fmt.Sprintf("integrations.%s", name), t.Container)...)
		for i, input := range t.Inputs {
			problems = append(problems, inputProblems(fmt.Sprintf("integrations.%s.inputs[%d]", name, i), input)...)
		}
	}

	for _, name := range slices.Sorted(maps.Keys(s.Ingestion)) {
		ing := s.Ingestion[name]
		if !validTriggerTypes[ing.Trigger.Type] {
			add(fmt.Sprintf("ingestion.%s.trigger.type", name),
				"ingestion.%s.trigger.type: must be one of schedule, startup, manual, webhook", name)
		}
		if ing.Container.Build != nil {
			problems = append(problems, buildProblems(fmt.Sprintf("ingestion.%s.container.build", name), ing.Container.Build)...)
		}
		for i, input := range ing.Inputs {
			problems = append(problems, inputProblems(fmt.Sprintf("ingestion.%s.inputs[%d]", name, i), input)...)
		}
	}

	return problems
}

// containerProblems validates a sidecar container. A nil container is the
// provider-backed form and has nothing to check.
func containerProblems(path string, c *ContainerConfig) []Problem {
	if c == nil {
		return nil
	}
	var problems []Problem
	if c.Build != nil {
		problems = append(problems, buildProblems(path+".container.build", c.Build)...)
	}
	if c.GPU != nil && c.GPU.Runtime != "" && c.GPU.Runtime != "cuda" && c.GPU.Runtime != "rocm" {
		problems = append(problems, Problem{
			Field:   path + ".container.gpu.runtime",
			Message: fmt.Sprintf("%s.container.gpu.runtime: must be one of cuda or rocm", path),
		})
	}
	return problems
}

// scopeProblems checks that a custom provider declares the section that
// references it. Whether a non-custom provider names a real backend is not
// decidable from the spec, so it belongs to the caller.
func scopeProblems(s *AstroSpec, section, entry, provider string) []Problem {
	custom, ok := s.Providers[provider]
	if !ok || scopeContains(custom.Scope, section) {
		return nil
	}
	return []Problem{{
		Field:   fmt.Sprintf("%s.%s.provider", section, entry),
		Message: fmt.Sprintf("%s %q: provider %q does not allow scope %q", sectionNoun(section), entry, provider, section),
	}}
}

// sectionNoun maps a YAML section name to the singular noun used in messages.
func sectionNoun(section string) string {
	switch section {
	case "models":
		return "model"
	case "integrations":
		return "integration"
	default:
		return section
	}
}

func inputProblems(path string, input Input) []Problem {
	var problems []Problem
	add := func(format string, args ...any) {
		problems = append(problems, Problem{Field: path, Message: fmt.Sprintf(format, args...)})
	}
	if input.Name == "" {
		add("%s: name is required", path)
	}
	if !validDatatypes[input.Datatype] {
		add("%s: datatype must be one of string, boolean, number, array, object (got %q)", path, input.Datatype)
	}
	if input.DisplayAs != "" && !validDisplayAs[input.DisplayAs] {
		add("%s: display-as must be one of short-text, long-text, select (got %q)", path, input.DisplayAs)
	}
	if input.DisplayAs == "select" && len(input.Options) == 0 {
		add("%s: options must be present and non-empty when display-as is select", path)
	}
	if input.DisplayAs == "select" && input.Default != "" && !slices.Contains(input.Options, input.Default) {
		add("%s: default %q must be one of the declared options", path, input.Default)
	}
	return problems
}

func buildProblems(path string, b *BuildConfig) []Problem {
	var problems []Problem
	if b.Context == "" {
		problems = append(problems, Problem{Field: path + ".context", Message: fmt.Sprintf("%s.context is required", path)})
	}
	if b.Dockerfile == "" {
		problems = append(problems, Problem{Field: path + ".dockerfile", Message: fmt.Sprintf("%s.dockerfile is required", path)})
	}
	return problems
}

// SecretDefaultViolations returns the names of all secret inputs that still
// carry a non-empty default value. These must be stripped before registration
// to avoid storing credentials in the registry.
func SecretDefaultViolations(s *AstroSpec) []string {
	var violations []string

	check := func(location, name, def string, secret bool) {
		if secret && def != "" {
			violations = append(violations, location+"."+name)
		}
	}

	for key, inp := range s.Inputs {
		check("inputs."+key, inp.Name, inp.Default, inp.Secret)
	}
	for _, inp := range s.Agent.Inputs {
		check("agent.inputs", inp.Name, inp.Default, inp.Secret)
	}
	for name, m := range s.Models {
		for _, inp := range m.Inputs {
			check("models."+name+".inputs", inp.Name, inp.Default, inp.Secret)
		}
	}
	for name, k := range s.Knowledge {
		for _, inp := range k.Inputs {
			check("knowledge."+name+".inputs", inp.Name, inp.Default, inp.Secret)
		}
	}
	for name, t := range s.Integrations {
		for _, inp := range t.Inputs {
			check("tools."+name+".inputs", inp.Name, inp.Default, inp.Secret)
		}
	}
	for name, ing := range s.Ingestion {
		for _, inp := range ing.Inputs {
			check("ingestion."+name+".inputs", inp.Name, inp.Default, inp.Secret)
		}
	}
	for name, prov := range s.Providers {
		for _, v := range prov.Variables {
			check("providers."+name+".variables", v.Name, v.Default, v.Secret)
		}
	}

	return violations
}

// DeprecationWarnings returns human-readable notices for deprecated spec usage.
// Callers (CLI validate/create) surface these without failing the parse.
func DeprecationWarnings(s *AstroSpec) []string {
	var warnings []string
	if s.Agent.AIGateway {
		warnings = append(warnings, "agent.astro_ai_gateway is deprecated; declare a model with `provider: gateway` (and list selectable models) instead")
	}
	return warnings
}

func scopeContains(scope []string, value string) bool {
	return slices.Contains(scope, value)
}
