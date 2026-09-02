package eval

import "testing"

func TestIsPresetRef(t *testing.T) {
	for _, ref := range PresetRefs() {
		if !IsPresetRef(ref) {
			t.Errorf("IsPresetRef(%q) = false, want true", ref)
		}
	}
	if IsPresetRef("preset/does-not-exist") {
		t.Error("IsPresetRef(unknown ref) = true, want false")
	}
}

func TestPresetKeyForEveryRef(t *testing.T) {
	for _, ref := range PresetRefs() {
		if presetKeyFor(ref) == "" {
			t.Errorf("presetKeyFor(%q) = \"\", want a non-empty key", ref)
		}
	}
}
