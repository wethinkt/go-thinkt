package db

import "strings"

// SourceFilter returns a SQL WHERE clause fragment and args for filtering by source.
//   - nil sources: no filter (all sources)
//   - empty sources: blocks all rows ("AND 1=0")
//   - non-empty: "AND <col> IN (?,?,...)" with args
func SourceFilter(sources []string, col string) (string, []any) {
	if sources == nil {
		return "", nil
	}
	if len(sources) == 0 {
		return "AND 1=0", nil
	}
	placeholders := strings.Repeat(",?", len(sources))[1:]
	args := make([]any, len(sources))
	for i, s := range sources {
		args[i] = s
	}
	return "AND " + col + " IN (" + placeholders + ")", args
}
