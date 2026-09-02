package spec

import "testing"

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"minimum length passes", "ab", false},
		{"typical name passes", "my-agent", false},
		{"maximum length passes", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"one character fails", "a", true},
		{"empty string fails", "", true},
		{"over maximum length fails", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{"reserved name fails", "astro", true},
		{"uppercase fails", "My-Agent", true},
		{"starts with digit fails", "1-agent", true},
		{"ends with hyphen fails", "agent-", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got := IsValidName(tt.input); got != !tt.wantErr {
				t.Errorf("IsValidName(%q) = %v, want %v", tt.input, got, !tt.wantErr)
			}
		})
	}
}

func TestSplitAgentName(t *testing.T) {
	tests := []struct {
		raw         string
		wantAccount string
		wantName    string
	}{
		{raw: "hackernews-sleuth", wantName: "hackernews-sleuth"},
		{raw: "@matt/hackernews-sleuth", wantAccount: "matt", wantName: "hackernews-sleuth"},
		// Without the @ there is no prefix, so the slash belongs to the name and
		// ValidateName rejects it rather than it reaching a registry path.
		{raw: "matt/hackernews-sleuth", wantName: "matt/hackernews-sleuth"},
		{raw: "@matt/", wantName: "@matt/"},
		{raw: "@/sleuth", wantName: "@/sleuth"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			account, name := SplitAgentName(tt.raw)
			if account != tt.wantAccount || name != tt.wantName {
				t.Errorf("SplitAgentName(%q) = (%q, %q), want (%q, %q)",
					tt.raw, account, name, tt.wantAccount, tt.wantName)
			}
		})
	}
}
