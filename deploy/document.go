package deploy

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	SchemaV1 = "deploy/v1"

	FileName = "astropods.deploy.yml"

	maxEnvironmentNameLen = 64
	maxDocumentBytes      = 64 * 1024
)

var ErrInvalidDocument = errors.New("invalid deploy document")

var knownAdapters = []string{"web", "slack", "mesh"}

type Document struct {
	Spec         string
	Environments map[string]Environment
}

type Environment struct {
	Branch     string
	Cluster    string
	Interfaces []string
	Public     *Public
	Variables  map[string]Variable
	Models     map[string]string
	Schedules  map[string]string
	Knowledge  map[string]string
	Resources  *Resources
}

type Public struct {
	Web    *bool
	Custom *bool
}

// Variable holds a literal Value or a Ref to an environment or account variable.
type Variable struct {
	Value string
	Ref   string
}

type Resources struct {
	Agent        *Compute
	Models       map[string]Compute
	Knowledge    map[string]Compute
	Integrations map[string]Compute
	Ingestion    map[string]Compute
}

type Compute struct {
	CPU    string
	Memory string
}

type rawDocument struct {
	Spec         string                    `yaml:"spec"`
	Environments map[string]rawEnvironment `yaml:"environments"`
}

type rawEnvironment struct {
	Branch     string              `yaml:"branch"`
	Cluster    string              `yaml:"cluster"`
	Interfaces []string            `yaml:"interfaces"`
	Public     *rawPublic          `yaml:"public"`
	Variables  map[string]Variable `yaml:"variables"`
	Models     map[string]string   `yaml:"models"`
	Schedules  map[string]string   `yaml:"schedules"`
	Knowledge  map[string]string   `yaml:"knowledge"`
	Resources  *rawResources       `yaml:"resources"`
}

type rawPublic struct {
	Web    *bool `yaml:"web"`
	Custom *bool `yaml:"custom"`
}

type rawResources struct {
	Agent        *rawCompute           `yaml:"agent"`
	Models       map[string]rawCompute `yaml:"models"`
	Knowledge    map[string]rawCompute `yaml:"knowledge"`
	Integrations map[string]rawCompute `yaml:"integrations"`
	Ingestion    map[string]rawCompute `yaml:"ingestion"`
}

type rawCompute struct {
	CPU    string `yaml:"cpu"`
	Memory string `yaml:"memory"`
}

func (v *Variable) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*v = Variable{Value: node.Value}
		return nil
	case yaml.MappingNode:
		if len(node.Content) != 2 || node.Content[0].Value != "ref" || node.Content[1].Kind != yaml.ScalarNode {
			return fmt.Errorf("line %d: a variable is a value or { ref: NAME }", node.Line)
		}
		ref := strings.TrimSpace(node.Content[1].Value)
		if ref == "" {
			return fmt.Errorf("line %d: ref must name a variable", node.Line)
		}
		*v = Variable{Ref: ref}
		return nil
	default:
		return fmt.Errorf("line %d: a variable is a value or { ref: NAME }", node.Line)
	}
}

// Parse validates an astropods.deploy.yml document offline. It does not check
// that clusters, variables, models, or knowledge stores exist.
func Parse(yamlText string) (Document, error) {
	if len(yamlText) > maxDocumentBytes {
		return Document{}, invalidDocument("document is %d bytes, exceeding %d", len(yamlText), maxDocumentBytes)
	}

	raw, err := decodeStrict(yamlText)
	if err != nil {
		return Document{}, err
	}
	if raw.Spec != SchemaV1 {
		return Document{}, invalidDocument("spec must be %q, got %q", SchemaV1, raw.Spec)
	}
	if len(raw.Environments) == 0 {
		return Document{}, invalidDocument("environments must name at least one environment")
	}

	envs := make(map[string]Environment, len(raw.Environments))
	for name, r := range raw.Environments {
		if trimmed := strings.TrimSpace(name); trimmed == "" || trimmed != name || len([]rune(name)) > maxEnvironmentNameLen {
			return Document{}, invalidDocument("environment %q: name must be 1 to %d characters with no surrounding spaces", name, maxEnvironmentNameLen)
		}
		env, err := normalizeEnvironment(r)
		if err != nil {
			return Document{}, invalidDocument("environment %q: %v", name, err)
		}
		envs[name] = env
	}
	return Document{Spec: raw.Spec, Environments: envs}, nil
}

// Names returns the environment names in sorted order.
func (d Document) Names() []string {
	names := make([]string, 0, len(d.Environments))
	for name := range d.Environments {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
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

func normalizeEnvironment(r rawEnvironment) (Environment, error) {
	if r.Branch != "" && strings.TrimSpace(r.Branch) != r.Branch {
		return Environment{}, fmt.Errorf("branch %q has surrounding spaces", r.Branch)
	}
	for _, a := range r.Interfaces {
		if !slices.Contains(knownAdapters, a) {
			return Environment{}, fmt.Errorf("interfaces: unknown interface %q, expected one of %s", a, strings.Join(knownAdapters, ", "))
		}
	}
	if r.Public != nil && r.Interfaces == nil {
		return Environment{}, errors.New("public needs interfaces in the same environment")
	}
	for name, v := range r.Variables {
		if name == "" {
			return Environment{}, errors.New("variables: a variable needs a name")
		}
		if v.Ref == "" && v.Value == "" {
			return Environment{}, fmt.Errorf("variables: %s has no value", name)
		}
	}

	env := Environment{
		Branch:     r.Branch,
		Cluster:    strings.TrimSpace(r.Cluster),
		Interfaces: r.Interfaces,
		Variables:  r.Variables,
		Models:     r.Models,
		Schedules:  r.Schedules,
		Knowledge:  r.Knowledge,
	}
	if r.Public != nil {
		env.Public = &Public{Web: r.Public.Web, Custom: r.Public.Custom}
	}
	if r.Resources != nil {
		res, err := normalizeResources(*r.Resources)
		if err != nil {
			return Environment{}, fmt.Errorf("resources: %w", err)
		}
		env.Resources = res
	}
	return env, nil
}

func normalizeResources(r rawResources) (*Resources, error) {
	res := &Resources{}
	if r.Agent != nil {
		c, err := normalizeCompute("agent", *r.Agent)
		if err != nil {
			return nil, err
		}
		res.Agent = &c
	}
	groups := []struct {
		label string
		in    map[string]rawCompute
		out   *map[string]Compute
	}{
		{"models", r.Models, &res.Models},
		{"knowledge", r.Knowledge, &res.Knowledge},
		{"integrations", r.Integrations, &res.Integrations},
		{"ingestion", r.Ingestion, &res.Ingestion},
	}
	for _, g := range groups {
		if len(g.in) == 0 {
			continue
		}
		*g.out = make(map[string]Compute, len(g.in))
		for name, rc := range g.in {
			c, err := normalizeCompute(g.label+"."+name, rc)
			if err != nil {
				return nil, err
			}
			(*g.out)[name] = c
		}
	}
	return res, nil
}

func normalizeCompute(label string, r rawCompute) (Compute, error) {
	c := Compute{CPU: strings.TrimSpace(r.CPU), Memory: strings.TrimSpace(r.Memory)}
	if c.CPU == "" && c.Memory == "" {
		return Compute{}, fmt.Errorf("%s sets neither cpu nor memory", label)
	}
	return c, nil
}

func invalidDocument(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidDocument, fmt.Sprintf(format, args...))
}
