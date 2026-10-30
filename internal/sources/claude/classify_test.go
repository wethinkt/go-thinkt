package claude

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// A queue-operation record without content (dequeue, remove, popAll) carries
// only a timestamp; it is bookkeeping, not an event a reader can act on.
func TestClassifyLineQueueOperationNeedsContent(t *testing.T) {
	if _, _, class := ClassifyLine([]byte(`{"type":"queue-operation","operation":"dequeue","timestamp":"2026-10-26T13:35:08.790Z"}`)); class != thinkt.ClassBookkeeping {
		t.Fatalf("content-less dequeue must be bookkeeping, got %v", class)
	}
	if _, _, class := ClassifyLine([]byte(`{"type":"queue-operation","operation":"remove","content":""}`)); class != thinkt.ClassBookkeeping {
		t.Fatalf("empty-content remove must be bookkeeping, got %v", class)
	}
	if _, _, class := ClassifyLine([]byte(`{"type":"queue-operation","operation":"enqueue","content":"  \n "}`)); class != thinkt.ClassBookkeeping {
		t.Fatalf("whitespace-only content must be bookkeeping, got %v", class)
	}
	if _, _, class := ClassifyLine([]byte(`{"type":"queue-operation","operation":"enqueue","content":"<cross-session-message>hi</cross-session-message>"}`)); class != thinkt.ClassEvent {
		t.Fatalf("enqueue with content must stay an event, got %v", class)
	}
	if thinkt.ParserVersion < 3 {
		t.Fatalf("dropping content-less queue records changes emitted entries; ParserVersion must be bumped, got %d", thinkt.ParserVersion)
	}
}

func TestClassifyType(t *testing.T) {
	cases := []struct {
		typ  EntryType
		att  string
		want thinkt.RecordClass
	}{
		{EntryTypeUser, "", thinkt.ClassTurn},
		{EntryTypeAssistant, "", thinkt.ClassTurn},
		{EntryTypeSystem, "", thinkt.ClassEvent},
		{EntryTypeQueueOperation, "", thinkt.ClassEvent},
		{EntryTypeAttachment, "hook_success", thinkt.ClassContext},
		{EntryTypeAttachment, "task_status", thinkt.ClassContext},
		{EntryTypeAttachment, "total_tokens_reminder", thinkt.ClassBookkeeping},
		{EntryTypeAttachment, "", thinkt.ClassBookkeeping},
		{EntryTypeAPIRequest, "", thinkt.ClassBookkeeping},
		{EntryTypeAPIRequestBlob, "", thinkt.ClassBookkeeping},
		{EntryTypeAPIRequestShape, "", thinkt.ClassBookkeeping},
		{EntryTypeMode, "", thinkt.ClassBookkeeping},
		{EntryTypePermissionMode, "", thinkt.ClassBookkeeping},
		{EntryTypeAtisLatch, "", thinkt.ClassBookkeeping},
		{EntryTypeAITitle, "", thinkt.ClassSessionState},
		{EntryTypeLastPrompt, "", thinkt.ClassSessionState},
		{EntryTypePRLink, "", thinkt.ClassSessionState},
		{EntryTypeRelocated, "", thinkt.ClassSessionState},
		{EntryTypeWorktreeState, "", thinkt.ClassSessionState},
		{EntryTypeCostState, "", thinkt.ClassSessionState},
		{EntryTypeFileHistorySnapshot, "", thinkt.ClassCheckpoint},
		{EntryTypeFileHistoryDelta, "", thinkt.ClassCheckpoint},
		{EntryTypeSummary, "", thinkt.ClassCheckpoint},
		{EntryTypeProgress, "", thinkt.ClassEvent},
		{EntryType("brand-new-thing"), "", thinkt.ClassUnknown},
		{EntryType(""), "", thinkt.ClassBookkeeping},
	}
	for _, c := range cases {
		if got := ClassifyType(c.typ, c.att); got != c.want {
			t.Errorf("%s/%s: got %v want %v", c.typ, c.att, got, c.want)
		}
	}
}

func TestClassifyLineReadsOnlyType(t *testing.T) {
	typ, att, class := ClassifyLine([]byte(`{"type":"attachment","attachment":{"type":"hook_success","stdout":"x"},"message":{"content":"not parsed"}}`))
	if typ != EntryTypeAttachment || att != "hook_success" || class != thinkt.ClassContext {
		t.Fatalf("got %s %s %v", typ, att, class)
	}
	if _, _, class := ClassifyLine([]byte(`{"type":"api-request-blob","hash":"h","message":{"role":"user","content":[]}}`)); class != thinkt.ClassBookkeeping {
		t.Fatal("blob must be bookkeeping")
	}
	if _, _, class := ClassifyLine([]byte(`not json`)); class != thinkt.ClassBookkeeping {
		t.Fatal("malformed line must be dropped by classifier")
	}
	if _, _, class := ClassifyLine([]byte(`{"type":"user","message":{"role":"user","content":"hi"}}`)); class != thinkt.ClassTurn {
		t.Fatal("user line must be a turn")
	}
}

func TestRecordClassIsEntry(t *testing.T) {
	for _, c := range []thinkt.RecordClass{thinkt.ClassTurn, thinkt.ClassEvent, thinkt.ClassContext, thinkt.ClassCheckpoint, thinkt.ClassUnknown} {
		if !c.IsEntry() {
			t.Errorf("%v should be entry", c)
		}
	}
	for _, c := range []thinkt.RecordClass{thinkt.ClassSessionState, thinkt.ClassBookkeeping} {
		if c.IsEntry() {
			t.Errorf("%v should not be entry", c)
		}
	}
}
