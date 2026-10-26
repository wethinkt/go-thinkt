package tui

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestEntryVisibleSecondaryUsesOther(t *testing.T) {
	entry := &thinkt.Entry{Role: thinkt.RoleUser, Kind: thinkt.KindMeta, Visibility: thinkt.VisibilitySecondary}
	f := NewRoleFilterSet()
	f.Other = false
	if f.EntryVisible(entry) {
		t.Fatal("secondary user entry must follow the Other toggle, not the User toggle")
	}
	f.Other = true
	if !f.EntryVisible(entry) {
		t.Fatal("Other reveals secondary entries")
	}
	normal := &thinkt.Entry{Role: thinkt.RoleUser}
	f.Other = false
	if !f.EntryVisible(normal) {
		t.Fatal("normal user entries are unaffected")
	}
}
