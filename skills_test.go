package spec

import (
	"encoding/json"
	"strings"
	"testing"
)

func skillsSpec(t *testing.T, skills string) (*AstroSpec, error) {
	t.Helper()
	return ParseSpecBytes([]byte(`spec: blueprint/v1
name: probe
agent:
  image: example/agent:1
` + skills))
}

func TestAnAgentWithoutSkillsDeclaresNone(t *testing.T) {
	parsed, err := skillsSpec(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Agent.Skills) != 0 {
		t.Errorf("a spec with no agent.skills parsed %d skills", len(parsed.Agent.Skills))
	}
}

func TestAgentSkillsParseInOrderWithTheirDescriptions(t *testing.T) {
	parsed, err := skillsSpec(t, `  skills:
    - name: github.issue.investigate
      description: Investigates a new GitHub issue.
    - name: notes_summarize-v2
`)
	if err != nil {
		t.Fatal(err)
	}
	want := []Skill{
		{Name: "github.issue.investigate", Description: "Investigates a new GitHub issue."},
		{Name: "notes_summarize-v2"},
	}
	if len(parsed.Agent.Skills) != len(want) {
		t.Fatalf("parsed %+v, want %+v", parsed.Agent.Skills, want)
	}
	for i := range want {
		if parsed.Agent.Skills[i] != want[i] {
			t.Errorf("skill %d is %+v, want %+v; order matters because the sidecar keeps the first entry for a name", i, parsed.Agent.Skills[i], want[i])
		}
	}
}

func TestAgentSkillsRejectInvalidEntries(t *testing.T) {
	cases := map[string]struct {
		yaml    string
		wantErr string
	}{
		"missing name": {
			yaml:    "  skills:\n    - description: No name\n",
			wantErr: "agent.skills[0].name",
		},
		"uppercase name": {
			yaml:    "  skills:\n    - name: GitHub.Issue\n",
			wantErr: "agent.skills[0].name: \"GitHub.Issue\" must be lowercase",
		},
		"name starting with a dot": {
			yaml:    "  skills:\n    - name: .hidden\n",
			wantErr: "agent.skills[0].name",
		},
		"name with a space": {
			yaml:    "  skills:\n    - name: \"issue triage\"\n",
			wantErr: "agent.skills[0].name",
		},
		"name over 128 characters": {
			yaml:    "  skills:\n    - name: " + strings.Repeat("a", 129) + "\n",
			wantErr: "agent.skills[0].name",
		},
		"reserved agent prefix": {
			yaml:    "  skills:\n    - name: agent.probe\n",
			wantErr: "agent.skills[0].name: \"agent.probe\" uses the reserved prefix \"agent.\"",
		},
		"duplicate name": {
			yaml:    "  skills:\n    - name: triage\n    - name: triage\n",
			wantErr: "agent.skills[1].name: \"triage\" is listed twice",
		},
		"description over 500 characters": {
			yaml:    "  skills:\n    - name: triage\n      description: " + strings.Repeat("a", 501) + "\n",
			wantErr: "agent.skills[0].description: must be at most 500 characters",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := skillsSpec(t, tc.yaml)
			if err == nil {
				t.Fatalf("spec parsed, want an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestASkillAtTheLengthLimitsIsAccepted(t *testing.T) {
	_, err := skillsSpec(t, "  skills:\n    - name: "+strings.Repeat("a", 128)+"\n      description: "+strings.Repeat("é", 500)+"\n")
	if err != nil {
		t.Errorf("a 128-character name and a 500-character description were refused: %v", err)
	}
}

func TestSchema_AgentSkillsDeclareTheNamePatternAndAWholeDescription(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(astroSchema, &doc); err != nil {
		t.Fatalf("unmarshal embedded schema: %v", err)
	}
	agent := doc["properties"].(map[string]any)["agent"].(map[string]any)["properties"].(map[string]any)
	skills, ok := agent["skills"].(map[string]any)
	if !ok {
		t.Fatal("agent.skills missing from the generated schema; regenerate it with go run ./cmd/generate-schema")
	}
	name := skills["items"].(map[string]any)["properties"].(map[string]any)["name"].(map[string]any)
	if got := name["pattern"]; got != validSkillName.String() {
		t.Errorf("schema pattern is %v, want the parser's %q so editors and the parser agree", got, validSkillName.String())
	}
	if desc, _ := name["description"].(string); !strings.Contains(desc, "Names starting with agent. are reserved") {
		t.Errorf("agent.skills[].name description %q is cut short; a comma in the tag truncates it", desc)
	}
}
