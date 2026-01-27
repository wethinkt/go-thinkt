package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
)

func TestFormatLong(t *testing.T) {
	projects := []claude.Project{
		{
			DisplayName:  "foo",
			FullPath:     "/Users/test/projects/foo",
			SessionCount: 3,
		},
		{
			DisplayName:  "bar",
			FullPath:     "/Users/test/projects/bar",
			SessionCount: 1,
		},
	}

	var buf bytes.Buffer
	f := NewProjectsFormatter(&buf)
	err := f.FormatLong(projects)
	if err != nil {
		t.Fatalf("FormatLong error: %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Should only have paths, one per line
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d: %q", len(lines), output)
	}

	if lines[0] != "/Users/test/projects/foo" {
		t.Errorf("expected first line to be path, got %q", lines[0])
	}
	if lines[1] != "/Users/test/projects/bar" {
		t.Errorf("expected second line to be path, got %q", lines[1])
	}

	// Should NOT contain session info
	if strings.Contains(output, "Sessions:") {
		t.Error("FormatLong should not include session count")
	}
}

func TestFormatLong_EmptyPath(t *testing.T) {
	projects := []claude.Project{
		{
			DisplayName:  "~",
			FullPath:     "", // empty path should become ~
			SessionCount: 1,
		},
	}

	var buf bytes.Buffer
	f := NewProjectsFormatter(&buf)
	err := f.FormatLong(projects)
	if err != nil {
		t.Fatalf("FormatLong error: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	if output != "~" {
		t.Errorf("expected '~', got %q", output)
	}
}

func TestFormatTree(t *testing.T) {
	projects := []claude.Project{
		{DisplayName: "foo", FullPath: "/Users/test/projects/foo", SessionCount: 3},
		{DisplayName: "bar", FullPath: "/Users/test/projects/bar", SessionCount: 1},
		{DisplayName: "baz", FullPath: "/Users/test/work/baz", SessionCount: 5},
	}

	var buf bytes.Buffer
	f := NewProjectsFormatter(&buf)
	err := f.FormatTree(projects)
	if err != nil {
		t.Fatalf("FormatTree error: %v", err)
	}

	output := buf.String()

	// Check parent directories
	if !strings.Contains(output, "/Users/test/projects/") {
		t.Error("expected /Users/test/projects/ parent in output")
	}
	if !strings.Contains(output, "/Users/test/work/") {
		t.Error("expected /Users/test/work/ parent in output")
	}

	// Check project names with counts
	if !strings.Contains(output, "foo (3)") {
		t.Error("expected 'foo (3)' in output")
	}
	if !strings.Contains(output, "bar (1)") {
		t.Error("expected 'bar (1)' in output")
	}
	if !strings.Contains(output, "baz (5)") {
		t.Error("expected 'baz (5)' in output")
	}

	// Check tree structure characters
	if !strings.Contains(output, "├──") {
		t.Error("expected tree branch character in output")
	}
	if !strings.Contains(output, "└──") {
		t.Error("expected tree end character in output")
	}
	if !strings.Contains(output, "│") {
		t.Error("expected tree continuation character in output")
	}
}

func TestFormatTree_ProperIndentation(t *testing.T) {
	projects := []claude.Project{
		{DisplayName: "alpha", FullPath: "/parent/alpha", SessionCount: 1},
		{DisplayName: "beta", FullPath: "/parent/beta", SessionCount: 2},
	}

	var buf bytes.Buffer
	f := NewProjectsFormatter(&buf)
	err := f.FormatTree(projects)
	if err != nil {
		t.Fatalf("FormatTree error: %v", err)
	}

	output := buf.String()
	lines := strings.Split(output, "\n")

	// First line should be the parent with └──
	if !strings.HasPrefix(lines[0], "└── /parent/") {
		t.Errorf("expected parent line to start with '└── /parent/', got %q", lines[0])
	}

	// Child lines should have proper indentation
	if !strings.HasPrefix(lines[1], "    ├── alpha") {
		t.Errorf("expected child line with '    ├── alpha', got %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "    └── beta") {
		t.Errorf("expected last child with '    └── beta', got %q", lines[2])
	}
}

func TestFormatTree_MultipleParents(t *testing.T) {
	projects := []claude.Project{
		{DisplayName: "a", FullPath: "/first/a", SessionCount: 1},
		{DisplayName: "b", FullPath: "/second/b", SessionCount: 1},
	}

	var buf bytes.Buffer
	f := NewProjectsFormatter(&buf)
	err := f.FormatTree(projects)
	if err != nil {
		t.Fatalf("FormatTree error: %v", err)
	}

	output := buf.String()

	// First parent should use ├── and │ for continuation
	if !strings.Contains(output, "├── /first/") {
		t.Error("expected first parent with ├──")
	}
	if !strings.Contains(output, "│   └── a") {
		t.Error("expected child under first parent with │ prefix")
	}

	// Last parent should use └── and spaces for continuation
	if !strings.Contains(output, "└── /second/") {
		t.Error("expected last parent with └──")
	}
	if !strings.Contains(output, "    └── b") {
		t.Error("expected child under last parent with space prefix")
	}
}

func TestFormatTree_EmptyPath(t *testing.T) {
	projects := []claude.Project{
		{DisplayName: "~", FullPath: "", SessionCount: 1},
	}

	var buf bytes.Buffer
	f := NewProjectsFormatter(&buf)
	err := f.FormatTree(projects)
	if err != nil {
		t.Fatalf("FormatTree error: %v", err)
	}

	output := buf.String()

	// Empty path should show under ~/
	if !strings.Contains(output, "~/") {
		t.Errorf("expected '~/' parent for empty path, got:\n%s", output)
	}
}

func TestFormatSummary_Default(t *testing.T) {
	projects := []claude.Project{
		{
			DisplayName:  "foo",
			FullPath:     "/Users/test/projects/foo",
			SessionCount: 3,
			LastModified: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			DisplayName:  "bar",
			FullPath:     "/Users/test/projects/bar",
			SessionCount: 1,
			LastModified: time.Date(2026, 1, 20, 14, 0, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	f := NewProjectsFormatter(&buf)
	err := f.FormatSummary(projects, "", SummaryOptions{})
	if err != nil {
		t.Fatalf("FormatSummary error: %v", err)
	}

	output := buf.String()

	// Check paths
	if !strings.Contains(output, "/Users/test/projects/foo") {
		t.Error("expected foo path in output")
	}
	if !strings.Contains(output, "/Users/test/projects/bar") {
		t.Error("expected bar path in output")
	}

	// Check session counts
	if !strings.Contains(output, "Sessions: 3") {
		t.Error("expected 'Sessions: 3' in output")
	}
	if !strings.Contains(output, "Sessions: 1") {
		t.Error("expected 'Sessions: 1' in output")
	}
}

func TestFormatSummary_SortByTimeDesc(t *testing.T) {
	projects := []claude.Project{
		{DisplayName: "old", FullPath: "/old", LastModified: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{DisplayName: "new", FullPath: "/new", LastModified: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)},
		{DisplayName: "mid", FullPath: "/mid", LastModified: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)},
	}

	var buf bytes.Buffer
	f := NewProjectsFormatter(&buf)
	err := f.FormatSummary(projects, "", SummaryOptions{SortBy: "time", Descending: true})
	if err != nil {
		t.Fatalf("FormatSummary error: %v", err)
	}

	output := buf.String()
	newIdx := strings.Index(output, "/new")
	midIdx := strings.Index(output, "/mid")
	oldIdx := strings.Index(output, "/old")

	if newIdx > midIdx || midIdx > oldIdx {
		t.Errorf("expected order: new, mid, old (descending by time)\ngot:\n%s", output)
	}
}

func TestFormatSummary_SortByTimeAsc(t *testing.T) {
	projects := []claude.Project{
		{DisplayName: "old", FullPath: "/old", LastModified: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{DisplayName: "new", FullPath: "/new", LastModified: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)},
		{DisplayName: "mid", FullPath: "/mid", LastModified: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)},
	}

	var buf bytes.Buffer
	f := NewProjectsFormatter(&buf)
	err := f.FormatSummary(projects, "", SummaryOptions{SortBy: "time", Descending: false})
	if err != nil {
		t.Fatalf("FormatSummary error: %v", err)
	}

	output := buf.String()
	oldIdx := strings.Index(output, "/old")
	midIdx := strings.Index(output, "/mid")
	newIdx := strings.Index(output, "/new")

	if oldIdx > midIdx || midIdx > newIdx {
		t.Errorf("expected order: old, mid, new (ascending by time)\ngot:\n%s", output)
	}
}

func TestFormatSummary_SortByNameAsc(t *testing.T) {
	projects := []claude.Project{
		{DisplayName: "zebra", FullPath: "/zebra"},
		{DisplayName: "alpha", FullPath: "/alpha"},
		{DisplayName: "mango", FullPath: "/mango"},
	}

	var buf bytes.Buffer
	f := NewProjectsFormatter(&buf)
	err := f.FormatSummary(projects, "", SummaryOptions{SortBy: "name", Descending: false})
	if err != nil {
		t.Fatalf("FormatSummary error: %v", err)
	}

	output := buf.String()
	alphaIdx := strings.Index(output, "/alpha")
	mangoIdx := strings.Index(output, "/mango")
	zebraIdx := strings.Index(output, "/zebra")

	if alphaIdx > mangoIdx || mangoIdx > zebraIdx {
		t.Errorf("expected order: alpha, mango, zebra (ascending by name)\ngot:\n%s", output)
	}
}

func TestFormatSummary_SortByNameDesc(t *testing.T) {
	projects := []claude.Project{
		{DisplayName: "zebra", FullPath: "/zebra"},
		{DisplayName: "alpha", FullPath: "/alpha"},
		{DisplayName: "mango", FullPath: "/mango"},
	}

	var buf bytes.Buffer
	f := NewProjectsFormatter(&buf)
	err := f.FormatSummary(projects, "", SummaryOptions{SortBy: "name", Descending: true})
	if err != nil {
		t.Fatalf("FormatSummary error: %v", err)
	}

	output := buf.String()
	zebraIdx := strings.Index(output, "/zebra")
	mangoIdx := strings.Index(output, "/mango")
	alphaIdx := strings.Index(output, "/alpha")

	if zebraIdx > mangoIdx || mangoIdx > alphaIdx {
		t.Errorf("expected order: zebra, mango, alpha (descending by name)\ngot:\n%s", output)
	}
}

func TestFormatSummary_CustomTemplate(t *testing.T) {
	projects := []claude.Project{
		{DisplayName: "foo", FullPath: "/test/foo", SessionCount: 5},
		{DisplayName: "bar", FullPath: "/test/bar", SessionCount: 2},
	}

	customTmpl := `{{range .}}{{.DisplayName}}:{{.SessionCount}}
{{end}}`

	var buf bytes.Buffer
	f := NewProjectsFormatter(&buf)
	err := f.FormatSummary(projects, customTmpl, SummaryOptions{})
	if err != nil {
		t.Fatalf("FormatSummary error: %v", err)
	}

	output := buf.String()
	expected := "foo:5\nbar:2\n"
	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func TestFormatSummary_InvalidTemplate(t *testing.T) {
	projects := []claude.Project{
		{DisplayName: "foo", FullPath: "/test/foo", SessionCount: 1},
	}

	var buf bytes.Buffer
	f := NewProjectsFormatter(&buf)
	err := f.FormatSummary(projects, "{{.Invalid", SummaryOptions{})
	if err == nil {
		t.Error("expected error for invalid template")
	}
}

func TestFormatSummary_EmptyPath(t *testing.T) {
	projects := []claude.Project{
		{DisplayName: "~", FullPath: "", SessionCount: 1},
	}

	var buf bytes.Buffer
	f := NewProjectsFormatter(&buf)
	err := f.FormatSummary(projects, "", SummaryOptions{})
	if err != nil {
		t.Fatalf("FormatSummary error: %v", err)
	}

	output := buf.String()
	if !strings.HasPrefix(output, "~\n") {
		t.Errorf("expected output to start with '~', got:\n%s", output)
	}
}

func TestFormatSummary_NoModified(t *testing.T) {
	projects := []claude.Project{
		{DisplayName: "test", FullPath: "/test", SessionCount: 2},
	}

	var buf bytes.Buffer
	f := NewProjectsFormatter(&buf)
	err := f.FormatSummary(projects, "", SummaryOptions{})
	if err != nil {
		t.Fatalf("FormatSummary error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "Modified:") {
		t.Error("should not contain Modified line when time is zero")
	}
}

func TestGroupByParent(t *testing.T) {
	projects := []claude.Project{
		{FullPath: "/a/x"},
		{FullPath: "/a/y"},
		{FullPath: "/b/z"},
		{FullPath: ""},
	}

	groups := groupByParent(projects)

	if len(groups) != 3 {
		t.Errorf("expected 3 groups, got %d", len(groups))
	}

	if len(groups["/a"]) != 2 {
		t.Errorf("expected 2 projects under /a, got %d", len(groups["/a"]))
	}

	if len(groups["/b"]) != 1 {
		t.Errorf("expected 1 project under /b, got %d", len(groups["/b"]))
	}

	if len(groups["~"]) != 1 {
		t.Errorf("expected 1 project under ~, got %d", len(groups["~"]))
	}
}

func TestSortedKeys(t *testing.T) {
	m := map[string][]claude.Project{
		"zebra": nil,
		"alpha": nil,
		"mango": nil,
	}

	keys := sortedKeys(m)

	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}

	if keys[0] != "alpha" || keys[1] != "mango" || keys[2] != "zebra" {
		t.Errorf("expected sorted order [alpha, mango, zebra], got %v", keys)
	}
}

func TestSortProjects_NameCaseInsensitive(t *testing.T) {
	projects := []claude.Project{
		{DisplayName: "Zebra"},
		{DisplayName: "alpha"},
		{DisplayName: "MANGO"},
	}

	sortProjects(projects, SummaryOptions{SortBy: "name", Descending: false})

	if projects[0].DisplayName != "alpha" {
		t.Errorf("expected alpha first, got %s", projects[0].DisplayName)
	}
	if projects[1].DisplayName != "MANGO" {
		t.Errorf("expected MANGO second, got %s", projects[1].DisplayName)
	}
	if projects[2].DisplayName != "Zebra" {
		t.Errorf("expected Zebra third, got %s", projects[2].DisplayName)
	}
}
