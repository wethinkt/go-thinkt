package thinkt

import "testing"

func TestIsSyntheticModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"<synthetic>", true},
		{"synthetic", true},
		{"<SYNTHETIC>", true},
		{"SYNTHETIC", true},
		{"<Synthetic>", true},
		{"Synthetic", true},
		{"  synthetic  ", true},
		{" <synthetic> ", true},
		{"\tsynthetic\n", true},

		{"", false},
		{"claude-opus-4-5-20251101", false},
		{"gpt-4", false},
		{"synthetic-v2", false},
		{"my-synthetic-model", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := IsSyntheticModel(tt.model); got != tt.want {
				t.Errorf("IsSyntheticModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestIsRealModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"claude-opus-4-5-20251101", true},
		{"gpt-4", true},
		{"synthetic-v2", true},

		{"", false},
		{"<synthetic>", false},
		{"synthetic", false},
		{"  SYNTHETIC  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := IsRealModel(tt.model); got != tt.want {
				t.Errorf("IsRealModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}
