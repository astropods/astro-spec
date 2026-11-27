package spec

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func parseAgent(t *testing.T, agentBody string) (*AstroSpec, error) {
	t.Helper()
	return ParseSpecBytes([]byte("spec: blueprint/v1\nname: test-agent\nagent:\n  image: test:latest\n" + agentBody))
}

func mustParseAgent(t *testing.T, agentBody string) *AstroSpec {
	t.Helper()
	s, err := parseAgent(t, agentBody)
	if err != nil {
		t.Fatalf("ParseSpecBytes(%q) = %v, want nil", agentBody, err)
	}
	return s
}

func TestAgentPort_ShorthandAndObjectDecodeToSameValue(t *testing.T) {
	want := AgentPort{Port: 3000}
	var fromYAML map[string]AgentPort
	if err := yaml.Unmarshal([]byte("a: 3000\nb: {port: 3000}\n"), &fromYAML); err != nil {
		t.Fatalf("yaml.Unmarshal = %v", err)
	}
	var fromJSON map[string]AgentPort
	if err := json.Unmarshal([]byte(`{"a": 3000, "b": {"port": 3000}}`), &fromJSON); err != nil {
		t.Fatalf("json.Unmarshal = %v", err)
	}
	for name, got := range map[string]AgentPort{"yaml shorthand": fromYAML["a"], "yaml object": fromYAML["b"], "json shorthand": fromJSON["a"], "json object": fromJSON["b"]} {
		if got != want {
			t.Errorf("%s decoded to %+v, want %+v", name, got, want)
		}
	}
}

func TestAgentPort_EffectiveProtocolDefaultsToHTTP(t *testing.T) {
	if got := (AgentPort{}).EffectiveProtocol(); got != PortProtocolHTTP {
		t.Errorf("EffectiveProtocol() with no protocol = %q, want %q", got, PortProtocolHTTP)
	}
	if got := (AgentPort{Protocol: PortProtocolTCP}).EffectiveProtocol(); got != PortProtocolTCP {
		t.Errorf("EffectiveProtocol() with tcp = %q, want %q", got, PortProtocolTCP)
	}
}

func TestFrontend_DecodesBoolAndPortName(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		json string
		want Frontend
	}{
		{name: "true", yaml: "frontend: true", json: `{"frontend": true}`, want: Frontend{Enabled: true}},
		{name: "false", yaml: "frontend: false", json: `{"frontend": false}`, want: Frontend{}},
		{name: "port name", yaml: "frontend: web", json: `{"frontend": "web"}`, want: Frontend{Enabled: true, Port: "web"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fromYAML, fromJSON Interfaces
			if err := yaml.Unmarshal([]byte(tt.yaml), &fromYAML); err != nil {
				t.Fatalf("yaml.Unmarshal(%q) = %v", tt.yaml, err)
			}
			if err := json.Unmarshal([]byte(tt.json), &fromJSON); err != nil {
				t.Fatalf("json.Unmarshal(%q) = %v", tt.json, err)
			}
			if fromYAML.Frontend != tt.want {
				t.Errorf("yaml %q decoded to %+v, want %+v", tt.yaml, fromYAML.Frontend, tt.want)
			}
			if fromJSON.Frontend != tt.want {
				t.Errorf("json %q decoded to %+v, want %+v", tt.json, fromJSON.Frontend, tt.want)
			}
		})
	}
}

func TestFrontend_RejectsEmptyNameAndNonScalar(t *testing.T) {
	for _, doc := range []string{`frontend: ""`, "frontend: [web]", "frontend: {port: web}"} {
		var i Interfaces
		if err := yaml.Unmarshal([]byte(doc), &i); err == nil {
			t.Errorf("yaml.Unmarshal(%q) = nil, want an error", doc)
		}
	}
}

func TestFrontend_MarshalRoundTripsThroughJSON(t *testing.T) {
	for _, f := range []Frontend{{}, {Enabled: true}, {Enabled: true, Port: "web"}} {
		data, err := json.Marshal(Interfaces{Frontend: f})
		if err != nil {
			t.Fatalf("json.Marshal(%+v) = %v", f, err)
		}
		var back Interfaces
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatalf("json.Unmarshal(%s) = %v", data, err)
		}
		if back.Frontend != f {
			t.Errorf("round trip of %+v through %s gave %+v", f, data, back.Frontend)
		}
	}
	data, _ := json.Marshal(Interfaces{})
	if strings.Contains(string(data), "frontend") {
		t.Errorf("json.Marshal of an unset frontend = %s, want the key omitted", data)
	}
}

func TestResolvedPorts(t *testing.T) {
	tests := []struct {
		name         string
		agent        string
		want         map[string]AgentPort
		wantFrontend string
		wantDefault  bool
	}{
		{
			name: "no ports and no frontend has no ports",
			want: map[string]AgentPort{},
		},
		{
			name:         "frontend true alone adds http 80",
			agent:        "  interfaces:\n    frontend: true\n",
			want:         map[string]AgentPort{"http": {Port: 80, Protocol: PortProtocolHTTP, Expose: true}},
			wantFrontend: "http",
			wantDefault:  true,
		},
		{
			name:         "frontend true with one exposed port picks it",
			agent:        "  interfaces:\n    frontend: true\n  ports:\n    web: {port: 3000, expose: true}\n    admin: 9000\n",
			want:         map[string]AgentPort{"web": {Port: 3000, Expose: true}, "admin": {Port: 9000}},
			wantFrontend: "web",
		},
		{
			name:         "frontend true with only internal ports adds http 80",
			agent:        "  interfaces:\n    frontend: true\n  ports:\n    admin: 9000\n",
			want:         map[string]AgentPort{"http": {Port: 80, Protocol: PortProtocolHTTP, Expose: true}, "admin": {Port: 9000}},
			wantFrontend: "http",
			wantDefault:  true,
		},
		{
			name:         "frontend name picks that port",
			agent:        "  interfaces:\n    frontend: web\n  ports:\n    web: {port: 3000, expose: true}\n",
			want:         map[string]AgentPort{"web": {Port: 3000, Expose: true}},
			wantFrontend: "web",
		},
		{
			name:  "exposed port without frontend is not the frontend",
			agent: "  ports:\n    hooks: {port: 3000, expose: true}\n",
			want:  map[string]AgentPort{"hooks": {Port: 3000, Expose: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mustParseAgent(t, tt.agent).Agent
			if got := c.ResolvedPorts(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ResolvedPorts() = %+v, want %+v", got, tt.want)
			}
			if got := c.FrontendPortName(); got != tt.wantFrontend {
				t.Errorf("FrontendPortName() = %q, want %q", got, tt.wantFrontend)
			}
			if got := c.UsesDefaultFrontendPort(); got != tt.wantDefault {
				t.Errorf("UsesDefaultFrontendPort() = %v, want %v", got, tt.wantDefault)
			}
		})
	}
}

func TestSortedPortNames_FrontendFirstThenByName(t *testing.T) {
	c := mustParseAgent(t, "  interfaces:\n    frontend: true\n  ports:\n    zeta: {port: 3000, expose: true}\n    beta: 9001\n    alpha: 9000\n").Agent
	want := []string{"zeta", "alpha", "beta"}
	if got := c.SortedPortNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("SortedPortNames() = %v, want %v", got, want)
	}
	if got := c.PrimaryPortName(); got != "zeta" {
		t.Errorf("PrimaryPortName() = %q, want the frontend port zeta", got)
	}
}

func TestProbePortName(t *testing.T) {
	tests := []struct {
		name  string
		agent string
		want  string
	}{
		{name: "frontend port", agent: "  interfaces:\n    frontend: true\n  ports:\n    a: 9000\n", want: "http"},
		{name: "only port", agent: "  ports:\n    a: 9000\n", want: "a"},
		{name: "two internal ports", agent: "  ports:\n    a: 9000\n    b: 9001\n", want: ""},
		{name: "no port", agent: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mustParseAgent(t, tt.agent).Agent.ProbePortName(); got != tt.want {
				t.Errorf("ProbePortName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPortEnv(t *testing.T) {
	tests := []struct {
		name  string
		agent string
		want  map[string]string
	}{
		{name: "no port", agent: "", want: map[string]string{}},
		{
			name:  "frontend default",
			agent: "  interfaces:\n    frontend: true\n",
			want:  map[string]string{"PORT": "80", "ASTRO_PORT_HTTP": "80"},
		},
		{
			name:  "declared frontend and internal port",
			agent: "  interfaces:\n    frontend: true\n  ports:\n    web: {port: 3000, expose: true}\n    admin-api: 9000\n",
			want:  map[string]string{"PORT": "3000", "ASTRO_PORT_WEB": "3000", "ASTRO_PORT_ADMIN_API": "9000"},
		},
		{
			name:  "no frontend sets no PORT",
			agent: "  ports:\n    admin: 9000\n",
			want:  map[string]string{"ASTRO_PORT_ADMIN": "9000"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mustParseAgent(t, tt.agent).Agent.PortEnv(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PortEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSpecBytes_PortRules(t *testing.T) {
	tests := []struct {
		name    string
		agent   string
		wantErr string
	}{
		{name: "rule 34: uppercase name", agent: "  ports:\n    Web: 3000\n", wantErr: "agent.ports.Web: name must be"},
		{name: "rule 34: name over 15 characters", agent: "  ports:\n    abcdefghijklmnop: 3000\n", wantErr: "name must be"},
		{name: "rule 35: port zero", agent: "  ports:\n    a: 0\n", wantErr: "port must be from 1 to 65535"},
		{name: "rule 35: port too high", agent: "  ports:\n    a: 70000\n", wantErr: "port must be from 1 to 65535"},
		{name: "rule 35: duplicate port", agent: "  ports:\n    a: 9000\n    b: 9000\n", wantErr: "port 9000 is already used"},
		{name: "rule 35: clash with the http 80 default", agent: "  interfaces:\n    frontend: true\n  ports:\n    a: 80\n", wantErr: "port 80 is already used"},
		{name: "rule 36: unknown protocol", agent: "  ports:\n    a: {port: 9000, protocol: udp}\n", wantErr: "protocol must be one of http, grpc, tcp"},
		{name: "rule 36: exposed tcp", agent: "  ports:\n    a: {port: 9000, protocol: tcp, expose: true}\n", wantErr: "an exposed port must use protocol http"},
		{name: "rule 37: frontend names a missing port", agent: "  interfaces:\n    frontend: web\n", wantErr: `"web" is not an entry of agent.ports`},
		{name: "rule 37: frontend names an internal port", agent: "  interfaces:\n    frontend: web\n  ports:\n    web: 3000\n", wantErr: "must set expose: true to serve the frontend"},
		{name: "rule 38: frontend true with two exposed ports", agent: "  interfaces:\n    frontend: true\n  ports:\n    a: {port: 3000, expose: true}\n    b: {port: 4000, expose: true}\n", wantErr: "several ports are exposed (a, b)"},
		{name: "rule 38: frontend true with an internal http port", agent: "  interfaces:\n    frontend: true\n  ports:\n    http: 8080\n", wantErr: "agent.ports.http exists but is not exposed"},
		{name: "rule 39: two exposed ports", agent: "  interfaces:\n    frontend: a\n  ports:\n    a: {port: 3000, expose: true}\n    b: {port: 4000, expose: true}\n", wantErr: "at most one port may set expose: true"},
		{name: "rule 40: healthcheck path with no port", agent: "  healthcheck:\n    path: /health\n", wantErr: "agent.healthcheck.path: needs a frontend port"},
		{name: "rule 40: healthcheck path with two internal ports", agent: "  ports:\n    a: 9000\n    b: 9001\n  healthcheck:\n    path: /health\n", wantErr: "agent.healthcheck.path: needs a frontend port"},
		{name: "rule 41: reserved input prefix", agent: "  inputs:\n    - name: ASTRO_PORT_X\n      datatype: string\n", wantErr: "reserves for agent ports"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseAgent(t, tt.agent)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ParseSpecBytes = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseSpecBytes_AcceptsValidPorts(t *testing.T) {
	for name, agent := range map[string]string{
		"frontend true alone":                  "  interfaces:\n    frontend: true\n",
		"frontend with exposed and internal":   "  interfaces:\n    frontend: true\n  ports:\n    web: {port: 3000, expose: true}\n    admin: {port: 9000, protocol: grpc}\n",
		"open port without frontend":           "  ports:\n    hooks: {port: 3000, expose: true}\n",
		"healthcheck path with one port":       "  ports:\n    a: 9000\n  healthcheck:\n    path: /health\n",
		"healthcheck path with frontend":       "  interfaces:\n    frontend: true\n  ports:\n    a: 9000\n  healthcheck:\n    path: /health\n",
		"frontend false with an internal port": "  interfaces:\n    frontend: false\n  ports:\n    a: 9000\n",
	} {
		t.Run(name, func(t *testing.T) {
			mustParseAgent(t, agent)
		})
	}
}

func TestDeprecationWarnings_DevFrontendPort(t *testing.T) {
	withPorts := mustParseAgent(t, "  interfaces:\n    frontend: true\n  ports:\n    web: {port: 3000, expose: true}\ndev:\n  interfaces:\n    frontend:\n      port: 5173\n")
	withoutPorts := mustParseAgent(t, "  interfaces:\n    frontend: true\ndev:\n  interfaces:\n    frontend:\n      port: 5173\n")
	if got := strings.Join(DeprecationWarnings(withPorts), "\n"); !strings.Contains(got, "ignored because agent.ports is set") {
		t.Errorf("DeprecationWarnings with agent.ports = %q, want the ignored notice", got)
	}
	if got := strings.Join(DeprecationWarnings(withoutPorts), "\n"); !strings.Contains(got, "set the port in agent.ports instead") {
		t.Errorf("DeprecationWarnings without agent.ports = %q, want the move-it notice", got)
	}
}

func TestSchema_AcceptsPortsAndNamedFrontend(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(Schema(), &schema); err != nil {
		t.Fatalf("json.Unmarshal(Schema()) = %v", err)
	}
	agent := schema["properties"].(map[string]any)["agent"].(map[string]any)["properties"].(map[string]any)
	if _, ok := agent["ports"]; !ok {
		t.Error("schema agent properties lack ports")
	}
	frontend := agent["interfaces"].(map[string]any)["properties"].(map[string]any)["frontend"].(map[string]any)
	if _, ok := frontend["oneOf"]; !ok {
		t.Errorf("schema agent.interfaces.frontend = %v, want a oneOf of boolean and string", frontend)
	}
}
