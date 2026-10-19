package spec

import (
	"encoding/json"
	"strings"
	"testing"
)

func jobsSpec(t *testing.T, jobs string) (*AstroSpec, error) {
	t.Helper()
	return ParseSpecBytes([]byte(`spec: blueprint/v1
name: probe
agent:
  image: example/agent:1
` + jobs))
}

func TestAnAgentWithoutJobsDeclaresNone(t *testing.T) {
	parsed, err := jobsSpec(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Agent.Jobs) != 0 {
		t.Errorf("a spec with no agent.jobs parsed %d jobs", len(parsed.Agent.Jobs))
	}
}

func TestAgentJobsParseInOrderWithTheirDescriptions(t *testing.T) {
	parsed, err := jobsSpec(t, `  jobs:
    - name: github.issue.investigate
      description: Investigates a new GitHub issue.
    - name: notes_summarize-v2
`)
	if err != nil {
		t.Fatal(err)
	}
	want := []Job{
		{Name: "github.issue.investigate", Description: "Investigates a new GitHub issue."},
		{Name: "notes_summarize-v2"},
	}
	if len(parsed.Agent.Jobs) != len(want) {
		t.Fatalf("parsed %+v, want %+v", parsed.Agent.Jobs, want)
	}
	for i := range want {
		if parsed.Agent.Jobs[i] != want[i] {
			t.Errorf("job %d is %+v, want %+v; order matters because the sidecar keeps the first entry for a name", i, parsed.Agent.Jobs[i], want[i])
		}
	}
}

func TestAgentJobsRejectInvalidEntries(t *testing.T) {
	cases := map[string]struct {
		yaml    string
		wantErr string
	}{
		"missing name": {
			yaml:    "  jobs:\n    - description: No name\n",
			wantErr: "agent.jobs[0].name",
		},
		"uppercase name": {
			yaml:    "  jobs:\n    - name: GitHub.Issue\n",
			wantErr: "agent.jobs[0].name: \"GitHub.Issue\" must be lowercase",
		},
		"name starting with a dot": {
			yaml:    "  jobs:\n    - name: .hidden\n",
			wantErr: "agent.jobs[0].name",
		},
		"name with a space": {
			yaml:    "  jobs:\n    - name: \"issue triage\"\n",
			wantErr: "agent.jobs[0].name",
		},
		"name over 128 characters": {
			yaml:    "  jobs:\n    - name: " + strings.Repeat("a", 129) + "\n",
			wantErr: "agent.jobs[0].name",
		},
		"reserved agent prefix": {
			yaml:    "  jobs:\n    - name: agent.probe\n",
			wantErr: "agent.jobs[0].name: \"agent.probe\" uses the reserved prefix \"agent.\"",
		},
		"duplicate name": {
			yaml:    "  jobs:\n    - name: triage\n    - name: triage\n",
			wantErr: "agent.jobs[1].name: \"triage\" is listed twice",
		},
		"description over 500 characters": {
			yaml:    "  jobs:\n    - name: triage\n      description: " + strings.Repeat("a", 501) + "\n",
			wantErr: "agent.jobs[0].description: must be at most 500 characters",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := jobsSpec(t, tc.yaml)
			if err == nil {
				t.Fatalf("spec parsed, want an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestAJobAtTheLengthLimitsIsAccepted(t *testing.T) {
	_, err := jobsSpec(t, "  jobs:\n    - name: "+strings.Repeat("a", 128)+"\n      description: "+strings.Repeat("é", 500)+"\n")
	if err != nil {
		t.Errorf("a 128-character name and a 500-character description were refused: %v", err)
	}
}

func TestSchema_AgentJobsDeclareTheNamePatternAndAWholeDescription(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(astroSchema, &doc); err != nil {
		t.Fatalf("unmarshal embedded schema: %v", err)
	}
	agent := doc["properties"].(map[string]any)["agent"].(map[string]any)["properties"].(map[string]any)
	jobs, ok := agent["jobs"].(map[string]any)
	if !ok {
		t.Fatal("agent.jobs missing from the generated schema; regenerate it with go run ./cmd/generate-schema")
	}
	name := jobs["items"].(map[string]any)["properties"].(map[string]any)["name"].(map[string]any)
	if got := name["pattern"]; got != validJobName.String() {
		t.Errorf("schema pattern is %v, want the parser's %q so editors and the parser agree", got, validJobName.String())
	}
	if desc, _ := name["description"].(string); !strings.Contains(desc, "Names starting with agent. are reserved") {
		t.Errorf("agent.jobs[].name description %q is cut short; a comma in the tag truncates it", desc)
	}
}

func TestValidateJobsAppliesTheParseRulesToADecodedSpec(t *testing.T) {
	if err := ValidateJobs([]Job{{Name: "github.issue.investigate"}}); err != nil {
		t.Errorf("a valid job was refused: %v", err)
	}
	err := ValidateJobs([]Job{{Name: "agent.probe"}})
	if err == nil || !strings.Contains(err.Error(), `uses the reserved prefix "agent."`) {
		t.Errorf("ValidateJobs(agent.probe) = %v, want the reserved-prefix error a server gets for a spec it decoded itself", err)
	}
}
