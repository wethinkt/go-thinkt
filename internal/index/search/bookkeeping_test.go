package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanFileSkipsBookkeepingLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	content := `{"type":"api-request-blob","hash":"h","message":{"role":"user","content":[{"type":"text","text":"needle in a prompt dump"}]}}
{"type":"user","uuid":"u1","message":{"role":"user","content":"needle in a real prompt"}}
{"type":"attachment","uuid":"at1","attachment":{"type":"total_tokens_reminder","text":"needle in a reminder"}}
{"role":"user","content":"needle in a kimi line"}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := NewMatcher("needle", false, false)
	if err != nil {
		t.Fatal(err)
	}
	out := make(chan RawMatch, 16)
	scanFile(context.Background(), Candidate{Path: path, Source: "claude"}, m, out)
	close(out)
	var lines []int
	for match := range out {
		if match.Role != "user" {
			t.Errorf("line %d: role %q", match.LineNum, match.Role)
		}
		lines = append(lines, match.LineNum)
	}
	if len(lines) != 2 || lines[0] != 2 || lines[1] != 4 {
		t.Fatalf("want matches on lines 2 and 4 only, got %v", lines)
	}
}

// The Claude classifier must only run on Claude files: Codex and Copilot
// lines also carry a top-level "type" and would classify as unknown.
func TestScanFileKeepsOtherSourcesWithTypedLines(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"codex":   `{"type":"event_msg","payload":{"type":"user_message","message":"needle in codex"}}` + "\n",
		"copilot": `{"type":"user.message","data":{"content":"needle in copilot"}}` + "\n",
	}
	for source, content := range cases {
		path := filepath.Join(dir, source+".jsonl")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		m, _ := NewMatcher("needle", false, false)
		out := make(chan RawMatch, 4)
		scanFile(context.Background(), Candidate{Path: path, Source: source}, m, out)
		close(out)
		if n := len(out); n != 1 {
			t.Errorf("%s: want 1 match, got %d", source, n)
		}
	}
}

// A line longer than bufio's default 64KB token must not end the scan.
func TestScanFileSurvivesLongLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	long := `{"type":"user","uuid":"u1","message":{"role":"user","content":"` + strings.Repeat("x", 70*1024) + `"}}` + "\n"
	content := long + `{"type":"user","uuid":"u2","message":{"role":"user","content":"needle after a long line"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ := NewMatcher("needle", false, false)
	out := make(chan RawMatch, 4)
	scanFile(context.Background(), Candidate{Path: path, Source: "claude"}, m, out)
	close(out)
	var lines []int
	for match := range out {
		lines = append(lines, match.LineNum)
	}
	if len(lines) != 1 || lines[0] != 2 {
		t.Fatalf("want a match on line 2, got %v", lines)
	}
}

// Secondary entries (reminders, hook output, notices, local command echoes)
// are hidden by default everywhere else; a search hit inside one is noise.
func TestScanFileSkipsSecondaryEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	content := `{"type":"user","uuid":"u1","isMeta":true,"message":{"role":"user","content":"<system-reminder>needle in a reminder</system-reminder>"}}
{"type":"attachment","uuid":"at1","attachment":{"type":"hook_success","hookName":"x","stdout":"needle in hook output"}}
{"type":"system","subtype":"informational","uuid":"s1","content":"needle in a notice"}
{"type":"user","uuid":"u2","message":{"role":"user","content":"<local-command-stdout>needle in a command echo</local-command-stdout>"}}
{"type":"assistant","uuid":"a1","message":{"id":"m","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"needle in a reply"}]}}
{"type":"system","subtype":"away_summary","uuid":"s2","content":"needle in an away summary"}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := NewMatcher("needle", false, false)
	if err != nil {
		t.Fatal(err)
	}
	out := make(chan RawMatch, 16)
	scanFile(context.Background(), Candidate{Path: path, Source: "claude"}, m, out)
	close(out)
	var lines []int
	for match := range out {
		lines = append(lines, match.LineNum)
	}
	if len(lines) != 2 || lines[0] != 5 || lines[1] != 6 {
		t.Fatalf("want matches on lines 5 and 6 only, got %v", lines)
	}
}

// A Codex AgentMessage item and its wire twin both carry the reply; only the
// item line is a hit.
func TestScanFileCodexSkipsWireAssistantTwin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "codex.jsonl")
	content := `{"timestamp":"2026-10-01T10:00:14Z","type":"event_msg","payload":{"type":"item_completed","turn_id":"turn-1","item":{"type":"AgentMessage","id":"msg-a2","content":[{"type":"Text","text":"needle fixed."}],"phase":"final_answer"}}}
{"timestamp":"2026-10-01T10:00:14Z","type":"response_item","payload":{"type":"message","id":"msg-a2","role":"assistant","content":[{"type":"output_text","text":"needle fixed."}],"phase":"final_answer"}}
{"timestamp":"2026-10-01T10:00:00Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"needle in developer prompt"}]}}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ := NewMatcher("needle", false, false)
	out := make(chan RawMatch, 8)
	scanFile(context.Background(), Candidate{Path: path, Source: "codex"}, m, out)
	close(out)
	var lines []int
	for match := range out {
		lines = append(lines, match.LineNum)
	}
	if len(lines) != 1 || lines[0] != 1 {
		t.Fatalf("want one match on line 1, got %v", lines)
	}
}

func kimiMatches(t *testing.T, content string) []int {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wire.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ := NewMatcher("needle", false, false)
	out := make(chan RawMatch, 8)
	scanFile(context.Background(), Candidate{Path: path, Source: "kimi"}, m, out)
	close(out)
	var lines []int
	for match := range out {
		lines = append(lines, match.LineNum)
	}
	return lines
}

// Some protocol 1.5 writers emit no appended records: the context message
// with an id is then the prompt and must be found.
func TestScanFileKimiFindsPromptWithoutAppendedTwin(t *testing.T) {
	lines := kimiMatches(t, `{"type":"metadata","protocol_version":"1.5"}
{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"needle prompt"}],"toolCalls":[],"origin":{"kind":"user"},"id":"m1"},"time":2}
`)
	if len(lines) != 1 || lines[0] != 2 {
		t.Fatalf("want one match on line 2, got %v", lines)
	}
}

// A steered message is written context-first, then appended with the same
// id: the hit belongs on the context line, which is the one the parser keeps.
func TestScanFileKimiSteeredPromptHitsContextLine(t *testing.T) {
	lines := kimiMatches(t, `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"needle steer"}],"toolCalls":[],"origin":{"kind":"user"},"id":"s1"},"time":1}
{"message":{"message":{"role":"user","content":[{"type":"text","text":"needle steer"}]},"meta":{"source":"input","origin":{"kind":"user"},"userMessageId":"s1"}},"type":"agent.message.appended","time":2}
`)
	if len(lines) != 1 || lines[0] != 1 {
		t.Fatalf("want one match on line 1, got %v", lines)
	}
}

// A Kimi prompt appears as an appended input record, its context twin, and
// in the batched assistant record; only the appended input line is a hit.
func TestScanFileKimiSkipsContextTwinAndBatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wire.jsonl")
	content := `{"message":{"message":{"role":"user","content":[{"type":"text","text":"needle prompt"}]},"meta":{"source":"input","origin":{"kind":"user"},"userMessageId":"m1"}},"type":"agent.message.appended","time":1}
{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"needle prompt"}],"toolCalls":[],"origin":{"kind":"user"},"id":"m1"},"time":2}
{"message":{"message":{"role":"assistant","content":[{"type":"text","text":"needle reply"}],"toolCalls":[]},"meta":{"source":"llm","messageId":"c1"}},"type":"agent.message.appended","time":3}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ := NewMatcher("needle", false, false)
	out := make(chan RawMatch, 8)
	scanFile(context.Background(), Candidate{Path: path, Source: "kimi"}, m, out)
	close(out)
	var lines []int
	for match := range out {
		lines = append(lines, match.LineNum)
	}
	if len(lines) != 1 || lines[0] != 1 {
		t.Fatalf("want one match on line 1, got %v", lines)
	}
}

func TestScanFileAntigravitySkipsHarnessLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcript_full.jsonl")
	content := `{"step_index":0,"type":"USER_INPUT","content":"<USER_REQUEST>\nneedle here\n</USER_REQUEST>"}
{"step_index":1,"type":"ERROR_MESSAGE","error":"needle in an error"}
{"step_index":2,"type":"EPHEMERAL_MESSAGE","content":"<EPHEMERAL_MESSAGE>needle reminder</EPHEMERAL_MESSAGE>"}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ := NewMatcher("needle", false, false)
	out := make(chan RawMatch, 8)
	scanFile(context.Background(), Candidate{Path: path, Source: "antigravity"}, m, out)
	close(out)
	var lines []int
	for match := range out {
		lines = append(lines, match.LineNum)
	}
	if len(lines) != 1 || lines[0] != 1 {
		t.Fatalf("want one match on line 1, got %v", lines)
	}
}
