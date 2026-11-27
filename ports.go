package spec

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/invopop/jsonschema"
	"gopkg.in/yaml.v3"
)

const (
	DefaultFrontendPortName = "http"
	DefaultFrontendPort     = 80
	PortEnvPrefix           = "ASTRO_PORT_"
	ExtURLEnvPrefix         = "ASTRO_EXT_URL_"
	PortProtocolHTTP        = "http"
	PortProtocolGRPC        = "grpc"
	PortProtocolTCP         = "tcp"
)

var validPortName = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,13}[a-z0-9])?$`)

func IsValidPortName(name string) bool {
	return validPortName.MatchString(name) && !strings.Contains(name, "--")
}

var validPortProtocols = map[string]bool{PortProtocolHTTP: true, PortProtocolGRPC: true, PortProtocolTCP: true}

type AgentPort struct {
	Port     int    `json:"port" yaml:"port"`
	Protocol string `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	Expose   bool   `json:"expose,omitempty" yaml:"expose,omitempty"`
}

func (p AgentPort) EffectiveProtocol() string {
	if p.Protocol == "" {
		return PortProtocolHTTP
	}
	return p.Protocol
}

func (p *AgentPort) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		var n int
		if err := value.Decode(&n); err != nil {
			return fmt.Errorf("port must be an integer or an object with port, protocol, and expose")
		}
		*p = AgentPort{Port: n}
		return nil
	}
	type agentPortAlias AgentPort
	var alias agentPortAlias
	if err := value.Decode(&alias); err != nil {
		return err
	}
	*p = AgentPort(alias)
	return nil
}

func (p *AgentPort) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*p = AgentPort{Port: n}
		return nil
	}
	type agentPortAlias AgentPort
	var alias agentPortAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*p = AgentPort(alias)
	return nil
}

func (AgentPort) JSONSchema() *jsonschema.Schema {
	props := jsonschema.NewProperties()
	props.Set("port", &jsonschema.Schema{
		Type:        "integer",
		Minimum:     json.Number("1"),
		Maximum:     json.Number("65535"),
		Description: "Container port the agent listens on",
	})
	props.Set("protocol", &jsonschema.Schema{
		Type:        "string",
		Enum:        []any{PortProtocolHTTP, PortProtocolGRPC, PortProtocolTCP},
		Description: "Protocol the port speaks. Default: http. An exposed port must use http",
	})
	props.Set("expose", &jsonschema.Schema{
		Type:        "boolean",
		Description: "Give this port a public ingress host. The frontend port sits behind the access settings; any other exposed port is open to the internet. Default: false",
	})
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{
				Type:        "integer",
				Minimum:     json.Number("1"),
				Maximum:     json.Number("65535"),
				Description: "Shorthand for {port: N}",
			},
			{
				Type:                 "object",
				Properties:           props,
				Required:             []string{"port"},
				AdditionalProperties: jsonschema.FalseSchema,
			},
		},
	}
}

type Frontend struct {
	Enabled bool
	Port    string
}

func (f Frontend) IsZero() bool { return !f.Enabled }

func (f *Frontend) set(raw any) error {
	switch v := raw.(type) {
	case bool:
		*f = Frontend{Enabled: v}
	case string:
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("agent.interfaces.frontend: a port name must not be empty")
		}
		*f = Frontend{Enabled: true, Port: v}
	default:
		return fmt.Errorf("agent.interfaces.frontend: must be true, false, or the name of an agent.ports entry")
	}
	return nil
}

func (f *Frontend) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return f.set(nil)
	}
	if value.Tag == "!!bool" {
		var b bool
		if err := value.Decode(&b); err != nil {
			return err
		}
		return f.set(b)
	}
	return f.set(value.Value)
}

func (f *Frontend) UnmarshalJSON(data []byte) error {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	return f.set(raw)
}

func (f Frontend) value() any {
	if f.Enabled && f.Port != "" {
		return f.Port
	}
	return f.Enabled
}

func (f Frontend) MarshalJSON() ([]byte, error) { return json.Marshal(f.value()) }

func (f Frontend) MarshalYAML() (any, error) { return f.value(), nil }

func (Frontend) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{Type: "boolean"},
			{Type: "string", Pattern: validPortName.String(), Description: "Name of the exposed agent.ports entry the frontend serves on"},
		},
		Description: "Agent serves its own web UI. A string names the exposed agent.ports entry Launch opens",
	}
}

func (c Container) ResolvedPorts() map[string]AgentPort {
	out := make(map[string]AgentPort, len(c.Ports)+1)
	exposed := false
	for name, p := range c.Ports {
		out[name] = p
		if p.Expose {
			exposed = true
		}
	}
	if c.HasFrontend() && c.Interfaces.Frontend.Port == "" && !exposed {
		if _, taken := out[DefaultFrontendPortName]; !taken {
			out[DefaultFrontendPortName] = AgentPort{Port: DefaultFrontendPort, Protocol: PortProtocolHTTP, Expose: true}
		}
	}
	return out
}

func (c Container) UsesDefaultFrontendPort() bool {
	if !c.HasFrontend() || c.Interfaces.Frontend.Port != "" {
		return false
	}
	for _, p := range c.Ports {
		if p.Expose {
			return false
		}
	}
	_, declared := c.Ports[DefaultFrontendPortName]
	return !declared
}

func (c Container) FrontendPortName() string {
	if !c.HasFrontend() {
		return ""
	}
	if c.Interfaces.Frontend.Port != "" {
		return c.Interfaces.Frontend.Port
	}
	var exposed []string
	for name, p := range c.ResolvedPorts() {
		if p.Expose {
			exposed = append(exposed, name)
		}
	}
	if len(exposed) == 1 {
		return exposed[0]
	}
	return ""
}

func (c Container) FrontendPort() (string, AgentPort, bool) {
	name := c.FrontendPortName()
	if name == "" {
		return "", AgentPort{}, false
	}
	p, ok := c.ResolvedPorts()[name]
	return name, p, ok
}

func (c Container) SortedPortNames() []string {
	ports := c.ResolvedPorts()
	frontend := c.FrontendPortName()
	names := make([]string, 0, len(ports))
	for name := range ports {
		if name != frontend {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if _, ok := ports[frontend]; ok && frontend != "" {
		names = append([]string{frontend}, names...)
	}
	return names
}

func (c Container) PrimaryPortName() string {
	names := c.SortedPortNames()
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func (c Container) ProbePortName() string {
	if name := c.FrontendPortName(); name != "" {
		return name
	}
	ports := c.ResolvedPorts()
	if len(ports) != 1 {
		return ""
	}
	for name := range ports {
		return name
	}
	return ""
}

func PortEnvName(portName string) string {
	return PortEnvPrefix + SanitizeEnvName(portName)
}

func ExtURLEnvName(portName string) string {
	return ExtURLEnvPrefix + SanitizeEnvName(portName)
}

func (c Container) PortEnv() map[string]string {
	ports := c.ResolvedPorts()
	env := make(map[string]string, len(ports)+1)
	for name, p := range ports {
		env[PortEnvName(name)] = strconv.Itoa(p.Port)
	}
	if _, p, ok := c.FrontendPort(); ok {
		env["PORT"] = strconv.Itoa(p.Port)
	}
	return env
}

func ValidatePorts(c Container) error {
	for name, p := range c.Ports {
		if !IsValidPortName(name) {
			return fmt.Errorf("agent.ports.%s: name must be 1 to 15 lowercase letters, digits, or single dashes, starting with a letter and ending with a letter or digit", name)
		}
		if p.Port < 1 || p.Port > 65535 {
			return fmt.Errorf("agent.ports.%s: port must be from 1 to 65535 (got %d)", name, p.Port)
		}
		if p.Protocol != "" && !validPortProtocols[p.Protocol] {
			return fmt.Errorf("agent.ports.%s: protocol must be one of http, grpc, tcp (got %q)", name, p.Protocol)
		}
		if p.Expose && p.EffectiveProtocol() != PortProtocolHTTP {
			return fmt.Errorf("agent.ports.%s: an exposed port must use protocol http", name)
		}
	}

	var exposed []string
	for name, p := range c.Ports {
		if p.Expose {
			exposed = append(exposed, name)
		}
	}
	sort.Strings(exposed)

	if c.HasFrontend() {
		if named := c.Interfaces.Frontend.Port; named != "" {
			p, ok := c.Ports[named]
			if !ok {
				return fmt.Errorf("agent.interfaces.frontend: %q is not an entry of agent.ports", named)
			}
			if !p.Expose {
				return fmt.Errorf("agent.interfaces.frontend: agent.ports.%s must set expose: true to serve the frontend", named)
			}
		} else {
			if len(exposed) > 1 {
				return fmt.Errorf("agent.interfaces.frontend: several ports are exposed (%s); set frontend to the name of the one Launch opens", strings.Join(exposed, ", "))
			}
			if len(exposed) == 0 {
				if _, taken := c.Ports[DefaultFrontendPortName]; taken {
					return fmt.Errorf("agent.interfaces.frontend: agent.ports.%s exists but is not exposed; set expose: true on it or name the frontend port", DefaultFrontendPortName)
				}
			}
		}
	}


	seen := map[int]string{}
	for _, name := range c.SortedPortNames() {
		p := c.ResolvedPorts()[name]
		if other, dup := seen[p.Port]; dup {
			return fmt.Errorf("agent.ports.%s: port %d is already used by agent.ports.%s", name, p.Port, other)
		}
		seen[p.Port] = name
	}

	if c.Healthcheck != nil && c.Healthcheck.Path != "" && c.ProbePortName() == "" {
		return fmt.Errorf("agent.healthcheck.path: needs a frontend port or exactly one agent.ports entry to probe; use healthcheck.test otherwise")
	}
	return nil
}
