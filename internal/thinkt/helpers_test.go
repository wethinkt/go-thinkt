package thinkt

import "testing"

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
		{1610612736, "1.5 GB"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			got := FormatBytes(tc.input)
			if got != tc.expected {
				t.Errorf("FormatBytes(%d) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}
