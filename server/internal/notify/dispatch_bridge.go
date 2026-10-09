package notify

import (
	"serverhub/internal/database"
	"serverhub/internal/delivery"
)

// DispatchSend routes one legacy event through the centralized delivery
// pipeline. The dedupe key is event + title: identical repeated failures
// collapse to one incident cycle, while distinct failures (different
// titles) track separately. Per-severity cooldowns, repeats, quiet hours
// and rate limits come from the shared policy (zero values = defaults).
func DispatchSend(db *database.DB, event, title, text string, channels []string, groupID uint) (delivery.Outcome, error) {
	return delivery.Dispatch(db, delivery.Input{
		Key:      "legacy:" + event + ":" + title,
		Severity: legacySeverity(event),
		Title:    title,
		Body:     text,
		Channels: channels,
		GroupID:  groupID,
	})
}
