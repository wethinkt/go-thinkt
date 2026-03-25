package db

import "testing"

func TestSourceFilter(t *testing.T) {
	tests := []struct {
		name    string
		sources []string
		col     string
		clause  string
		args    int
	}{
		{"nil sources", nil, "p.source", "", 0},
		{"empty sources", []string{}, "p.source", "AND 1=0", 0},
		{"one source", []string{"claude"}, "p.source", "AND p.source IN (?)", 1},
		{"two sources", []string{"claude", "kimi"}, "p.source", "AND p.source IN (?,?)", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clause, args := SourceFilter(tt.sources, tt.col)
			if clause != tt.clause {
				t.Errorf("clause = %q, want %q", clause, tt.clause)
			}
			if len(args) != tt.args {
				t.Errorf("args len = %d, want %d", len(args), tt.args)
			}
		})
	}
}
