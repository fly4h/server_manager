package activity

import (
	"database/sql"
	"encoding/json"
)

// Entry is one audit-log record. Never put secrets in Metadata.
type Entry struct {
	ID         int64  `json:"id"`
	Username   string `json:"username"`
	Action     string `json:"action"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// Logger writes activity entries.
type Logger struct{ db *sql.DB }

func NewLogger(db *sql.DB) *Logger { return &Logger{db: db} }

// Log records an action. metadata must not contain secrets.
func (l *Logger) Log(userID int64, username, action, targetType, targetID string, metadata map[string]string) {
	meta := "{}"
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			meta = string(b)
		}
	}
	_, _ = l.db.Exec(
		`INSERT INTO activity_logs (user_id, username, action, target_type, target_id, metadata) VALUES (?,?,?,?,?,?)`,
		userID, username, action, targetType, targetID, meta)
}

// List returns the most recent entries.
func (l *Logger) List(limit int) ([]Entry, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := l.db.Query(
		`SELECT id, COALESCE(username,''), action, COALESCE(target_type,''), COALESCE(target_id,''), COALESCE(metadata,'{}'), created_at
		 FROM activity_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		var e Entry
		var meta string
		if err := rows.Scan(&e.ID, &e.Username, &e.Action, &e.TargetType, &e.TargetID, &meta, &e.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(meta), &e.Metadata)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
