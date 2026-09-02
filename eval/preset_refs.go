package eval

// Preset evaluator references a builder may use in an EVALUATION.yaml "ref"
// field. Each preset's prompt text lives in astro-server's evalpreset
// package; this file carries only the ref name and the evaluator key it
// resolves to.
const (
	RefExposedPII                  = "preset/exposed-pii"
	RefLeakedCredentials           = "preset/leaked-credentials" //nolint:gosec // G101 matches the name; the value is a preset reference
	RefDisclosedSystemInstructions = "preset/disclosed-system-instructions"
	RefUnnecessaryToolCall         = "preset/unnecessary-tool-call"
	RefClaimGrounding              = "preset/claim-grounding"
	RefUserSentiment               = "preset/user-sentiment"
)

var presetRefOrder = []string{
	RefExposedPII,
	RefLeakedCredentials,
	RefDisclosedSystemInstructions,
	RefUnnecessaryToolCall,
	RefClaimGrounding,
	RefUserSentiment,
}

var presetKeys = map[string]string{
	RefExposedPII:                  "exposed_pii",
	RefLeakedCredentials:           "leaked_credentials",
	RefDisclosedSystemInstructions: "disclosed_system_instructions",
	RefUnnecessaryToolCall:         "unnecessary_tool_call",
	RefClaimGrounding:              "claim_grounding",
	RefUserSentiment:               "user_sentiment",
}

// IsPresetRef reports whether ref names a known preset evaluator.
func IsPresetRef(ref string) bool {
	_, ok := presetKeys[ref]
	return ok
}

// PresetRefs returns every preset evaluator reference, in declared order.
func PresetRefs() []string {
	return append([]string(nil), presetRefOrder...)
}

// PresetKey returns the evaluator key a preset ref resolves to.
func PresetKey(ref string) (string, bool) {
	key, ok := presetKeys[ref]
	return key, ok
}

func presetKeyFor(ref string) string {
	return presetKeys[ref]
}
