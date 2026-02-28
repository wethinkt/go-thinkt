package gemini

import (
	"encoding/json"
	"testing"
)

func TestMessageUnmarshalJSON_StringContent(t *testing.T) {
	data := `{"id":"1","content":"hello world"}`
	var msg Message
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Content != "hello world" {
		t.Errorf("got %q, want %q", msg.Content, "hello world")
	}
}

func TestMessageUnmarshalJSON_ArrayContent(t *testing.T) {
	data := `{"id":"1","content":[{"text":"part one"},{"text":"part two"}]}`
	var msg Message
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		t.Fatal(err)
	}
	want := "part one\npart two"
	if msg.Content != want {
		t.Errorf("got %q, want %q", msg.Content, want)
	}
}

func TestMessageUnmarshalJSON_NullContent(t *testing.T) {
	data := `{"id":"1","content":null}`
	var msg Message
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Content != "" {
		t.Errorf("got %q, want empty", msg.Content)
	}
}

func TestMessageUnmarshalJSON_PreservesOtherFields(t *testing.T) {
	data := `{"id":"abc","type":"user","content":"test","model":"gemini-2.0"}`
	var msg Message
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.ID != "abc" || msg.Type != "user" || msg.Model != "gemini-2.0" || msg.Content != "test" {
		t.Errorf("fields not preserved: %+v", msg)
	}
}
