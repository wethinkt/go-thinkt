package db

import (
	"time"
)

// AttachmentRow points at an attachment entry in a session: enough to list
// hook output, loaded instructions, invoked skills, or task status and jump
// to the entry by index. No text body is stored.
type AttachmentRow struct {
	SessionID  string
	EntryIndex int
	Kind       string
	Label      string
	Timestamp  time.Time
}

// ListAttachments returns a session's attachment rows in entry order. An empty
// kind returns all kinds.
func (d *DB) ListAttachments(sessionID, kind string) ([]AttachmentRow, error) {
	query := `SELECT session_id, entry_index, kind, COALESCE(label, ''), COALESCE(timestamp, '') FROM entry_attachments WHERE session_id = ?`
	args := []any{sessionID}
	if kind != "" {
		query += ` AND kind = ?`
		args = append(args, kind)
	}
	query += ` ORDER BY entry_index`
	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AttachmentRow
	for rows.Next() {
		var row AttachmentRow
		var ts string
		if err := rows.Scan(&row.SessionID, &row.EntryIndex, &row.Kind, &row.Label, &ts); err != nil {
			return nil, err
		}
		if ts != "" {
			row.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
