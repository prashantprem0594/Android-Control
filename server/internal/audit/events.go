package audit

import (
	"context"
	"database/sql"
	"fmt"
)

type Event struct {
	ID         int64  `json:"id"`
	OccurredAt string `json:"occurredAt"`
	ActorType  string `json:"actorType"`
	ActorID    string `json:"actorId"`
	Action     string `json:"action"`
	TargetID   string `json:"targetId"`
	Reason     string `json:"reason,omitempty"`
}

// List reads an ascending, bounded page for one authenticated parent.
// Unattributed events are retained for operators and never exposed to parents.
func List(ctx context.Context, db *sql.DB, parentID string, after int64, limit int) ([]Event, error) {
	if db == nil || parentID == "" || after < 0 || limit < 1 || limit > 100 {
		return nil, fmt.Errorf("invalid audit page")
	}
	rows, err := db.QueryContext(ctx, `SELECT id,occurred_at,actor_type,actor_id,action,target_id,reason
 FROM audit_events WHERE parent_account_id=? AND id>? ORDER BY id LIMIT ?`, parentID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]Event, 0)
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.OccurredAt, &e.ActorType, &e.ActorID, &e.Action, &e.TargetID, &e.Reason); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
