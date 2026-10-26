package claude

import "testing"

func TestClassifyType(t *testing.T) {
	cases := []struct {
		typ  EntryType
		att  string
		want RecordClass
	}{
		{EntryTypeUser, "", ClassTurn},
		{EntryTypeAssistant, "", ClassTurn},
		{EntryTypeSystem, "", ClassEvent},
		{EntryTypeQueueOperation, "", ClassEvent},
		{EntryTypeAttachment, "hook_success", ClassContext},
		{EntryTypeAttachment, "task_status", ClassContext},
		{EntryTypeAttachment, "total_tokens_reminder", ClassBookkeeping},
		{EntryTypeAttachment, "", ClassBookkeeping},
		{EntryTypeAPIRequest, "", ClassBookkeeping},
		{EntryTypeAPIRequestBlob, "", ClassBookkeeping},
		{EntryTypeAPIRequestShape, "", ClassBookkeeping},
		{EntryTypeMode, "", ClassBookkeeping},
		{EntryTypePermissionMode, "", ClassBookkeeping},
		{EntryTypeAtisLatch, "", ClassBookkeeping},
		{EntryTypeAITitle, "", ClassSessionState},
		{EntryTypeLastPrompt, "", ClassSessionState},
		{EntryTypePRLink, "", ClassSessionState},
		{EntryTypeRelocated, "", ClassSessionState},
		{EntryTypeWorktreeState, "", ClassSessionState},
		{EntryTypeCostState, "", ClassSessionState},
		{EntryTypeFileHistorySnapshot, "", ClassCheckpoint},
		{EntryTypeFileHistoryDelta, "", ClassCheckpoint},
		{EntryTypeSummary, "", ClassCheckpoint},
		{EntryTypeProgress, "", ClassEvent},
		{EntryType("brand-new-thing"), "", ClassUnknown},
		{EntryType(""), "", ClassBookkeeping},
	}
	for _, c := range cases {
		if got := ClassifyType(c.typ, c.att); got != c.want {
			t.Errorf("%s/%s: got %v want %v", c.typ, c.att, got, c.want)
		}
	}
}

func TestClassifyLineReadsOnlyType(t *testing.T) {
	typ, att, class := ClassifyLine([]byte(`{"type":"attachment","attachment":{"type":"hook_success","stdout":"x"},"message":{"content":"not parsed"}}`))
	if typ != EntryTypeAttachment || att != "hook_success" || class != ClassContext {
		t.Fatalf("got %s %s %v", typ, att, class)
	}
	if _, _, class := ClassifyLine([]byte(`{"type":"api-request-blob","hash":"h","message":{"role":"user","content":[]}}`)); class != ClassBookkeeping {
		t.Fatal("blob must be bookkeeping")
	}
	if _, _, class := ClassifyLine([]byte(`not json`)); class != ClassBookkeeping {
		t.Fatal("malformed line must be dropped by classifier")
	}
	if _, _, class := ClassifyLine([]byte(`{"type":"user","message":{"role":"user","content":"hi"}}`)); class != ClassTurn {
		t.Fatal("user line must be a turn")
	}
}

func TestRecordClassIsEntry(t *testing.T) {
	for _, c := range []RecordClass{ClassTurn, ClassEvent, ClassContext, ClassCheckpoint, ClassUnknown} {
		if !c.IsEntry() {
			t.Errorf("%v should be entry", c)
		}
	}
	for _, c := range []RecordClass{ClassSessionState, ClassBookkeeping} {
		if c.IsEntry() {
			t.Errorf("%v should not be entry", c)
		}
	}
}
