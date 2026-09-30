package spec

import (
	"strings"
	"testing"
)

func connectionsSpec(t *testing.T, connections string) (*AstroSpec, error) {
	t.Helper()
	return ParseSpecBytes([]byte(`spec: blueprint/v1
name: probe
agent:
  image: example/agent:1
` + connections))
}

func TestAnAgentWithoutConnectionsDeclaresNone(t *testing.T) {
	parsed, err := connectionsSpec(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Connections) != 0 {
		t.Errorf("a spec with no connections section parsed %d", len(parsed.Connections))
	}
}

func TestAConnectionParsesEveryField(t *testing.T) {
	parsed, err := connectionsSpec(t, `connections:
  - provider: github
    scopes: [repo, "read:org"]
    required: false
    reason: Open pull requests in your repositories
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Connections) != 1 {
		t.Fatalf("parsed %d connections, want 1", len(parsed.Connections))
	}
	c := parsed.Connections[0]
	if c.Provider != "github" {
		t.Errorf("provider is %q", c.Provider)
	}
	if strings.Join(c.Scopes, ",") != "repo,read:org" {
		t.Errorf("scopes are %v", c.Scopes)
	}
	if c.IsRequired() {
		t.Error("required: false parsed as required")
	}
	if c.Reason != "Open pull requests in your repositories" {
		t.Errorf("reason is %q", c.Reason)
	}
}

func TestAConnectionIsRequiredWhenRequiredIsOmitted(t *testing.T) {
	parsed, err := connectionsSpec(t, `connections:
  - provider: github
    reason: Read your repositories
`)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Connections[0].IsRequired() {
		t.Error("a connection with no required field must default to required")
	}
}

func TestConnectionsRejectInvalidEntries(t *testing.T) {
	cases := map[string]struct {
		yaml    string
		wantErr string
	}{
		"missing provider": {
			yaml:    "connections:\n  - reason: Read your repositories\n",
			wantErr: "connections[0].provider",
		},
		"uppercase provider": {
			yaml:    "connections:\n  - provider: GitHub\n    reason: Read your repositories\n",
			wantErr: "connections[0].provider",
		},
		"duplicate provider": {
			yaml:    "connections:\n  - provider: github\n    reason: One\n  - provider: github\n    reason: Two\n",
			wantErr: "connections[1].provider: \"github\" is declared more than once",
		},
		"missing reason": {
			yaml:    "connections:\n  - provider: github\n",
			wantErr: "connections[0].reason: required",
		},
		"blank reason": {
			yaml:    "connections:\n  - provider: github\n    reason: \"  \"\n",
			wantErr: "connections[0].reason: required",
		},
		"reason too long": {
			yaml:    "connections:\n  - provider: github\n    reason: " + strings.Repeat("a", 121) + "\n",
			wantErr: "connections[0].reason: must be 120 characters or fewer",
		},
		"scope with a space": {
			yaml:    "connections:\n  - provider: github\n    reason: Read\n    scopes: [\"repo read:org\"]\n",
			wantErr: "connections[0].scopes[0]",
		},
		"empty scope": {
			yaml:    "connections:\n  - provider: github\n    reason: Read\n    scopes: [\"\"]\n",
			wantErr: "connections[0].scopes[0]",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := connectionsSpec(t, tc.yaml)
			if err == nil {
				t.Fatalf("spec parsed, want an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestAReasonOfExactly120CharactersIsAccepted(t *testing.T) {
	_, err := connectionsSpec(t, "connections:\n  - provider: github\n    reason: "+strings.Repeat("é", 120)+"\n")
	if err != nil {
		t.Errorf("a 120-character reason was rejected: %v", err)
	}
}
