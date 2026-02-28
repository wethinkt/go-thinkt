package i18n

import (
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	Init("en")
	now := time.Now()
	tests := []struct {
		since time.Time
		want  string
	}{
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-90 * time.Second), "1 min ago"},
		{now.Add(-5 * time.Minute), "5 mins ago"},
		{now.Add(-90 * time.Minute), "1 hour ago"},
		{now.Add(-3 * time.Hour), "3 hours ago"},
		{now.Add(-36 * time.Hour), "1 day ago"},
		{now.Add(-72 * time.Hour), "3 days ago"},
	}
	for _, tt := range tests {
		got := RelativeTime(tt.since)
		if got != tt.want {
			t.Errorf("RelativeTime(%v ago) = %q, want %q", now.Sub(tt.since), got, tt.want)
		}
	}
}

func TestRelativeTimeShort(t *testing.T) {
	Init("en")
	now := time.Now()
	tests := []struct {
		since time.Time
		want  string
	}{
		{time.Time{}, ""},
		{now.Add(-3 * time.Hour), "today"},
		{now.Add(-36 * time.Hour), "1d ago"},
		{now.Add(-5 * 24 * time.Hour), "5d ago"},
		{now.Add(-60 * 24 * time.Hour), "2mo ago"},
		{now.Add(-400 * 24 * time.Hour), "1y ago"},
	}
	for _, tt := range tests {
		got := RelativeTimeShort(tt.since)
		if got != tt.want {
			t.Errorf("RelativeTimeShort(%v ago) = %q, want %q", now.Sub(tt.since), got, tt.want)
		}
	}
}
