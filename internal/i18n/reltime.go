package i18n

import (
	"fmt"
	"time"
)

// RelativeTime returns a human-readable relative time string (long form).
func RelativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return T("common.time.justNow", "just now")
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return T("common.time.oneMinAgo", "1 min ago")
		}
		return Tf("common.time.minsAgo", "%d mins ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return T("common.time.oneHourAgo", "1 hour ago")
		}
		return Tf("common.time.hoursAgo", "%d hours ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return T("common.time.oneDayAgo", "1 day ago")
		}
		return Tf("common.time.daysAgo", "%d days ago", days)
	}
}

// RelativeTimeShort returns a compact relative time string for TUI lists.
func RelativeTimeShort(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < 24*time.Hour:
		return T("common.time.short.today", "today")
	case d < 48*time.Hour:
		return T("common.time.short.oneDayAgo", "1d ago")
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
	}
}
