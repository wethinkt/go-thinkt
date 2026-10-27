package codex

import "testing"

func TestIsSearchableCodex(t *testing.T) {
	cases := map[string]bool{
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"a","content":[{"type":"Text","text":"needle"}]}}}`:                                                            true,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"needle"}]}}`:                                                                             false,
		`{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"needle"}]}}`:                                                                              false,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"needle"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["user.text"]}}}`: true,
		`{"type":"response_item","payload":{"type":"custom_tool_call","call_id":"c","name":"exec","input":"needle"}}`:                                                                                             false,
		`{"type":"response_item","payload":{"type":"function_call","call_id":"c","name":"exec_command","arguments":"{\"cmd\":\"needle\"}"}}`:                                                                      true,
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"e","command":["needle"],"source":"unified_exec_startup","exit_code":0,"duration":{"secs":0,"nanos":0}}}}`: true,
		`{"type":"event_msg","payload":{"type":"user_message","message":"needle"}}`:                                                                                                                               true,
		`{"type":"token_usage_record","payload":{"usage":{"input_tokens":1}}}`:                                                                                                                                    false,
		`{"type":"mystery","payload":{"x":"needle"}}`: false,
		`not json`: false,
	}
	for line, want := range cases {
		if got := IsSearchable([]byte(line)); got != want {
			t.Errorf("IsSearchable(%s) = %v, want %v", line, got, want)
		}
	}
}
