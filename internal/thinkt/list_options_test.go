package thinkt

import "testing"

func TestWithSubagentsSetsIncludeSubagents(t *testing.T) {
	if ResolveListOptions(nil).IncludeSubagents {
		t.Fatal("default must exclude subagents")
	}
	if !ResolveListOptions([]ListSessionsOption{WithSubagents()}).IncludeSubagents {
		t.Fatal("WithSubagents must set IncludeSubagents")
	}
}
