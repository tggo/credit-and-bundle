package storages

import (
	"context"
	"encoding/json"
	"time"

	"bundle-credit-test/internal/billing/models"
	"bundle-credit-test/internal/platform/db"
)

// RecordEvent stores the event in the inbox; fresh=false when this event_id was seen before.
func (s *Store) RecordEvent(ctx context.Context, q db.Q, ev models.Event, at time.Time) (bool, error) {
	payload, _ := json.Marshal(ev)
	tag, err := q.Exec(ctx, `INSERT INTO webhook_events (event_id, type, provider_sub_ref, payload, status, received_at)
		VALUES ($1,$2,$3,$4,'applied',$5) ON CONFLICT (event_id) DO NOTHING`, ev.EventID, ev.Type, ev.SubRef, payload, at)
	return err == nil && tag.RowsAffected() == 1, err
}

func (s *Store) SetEventResult(ctx context.Context, q db.Q, eventID, status, reason string) error {
	_, err := q.Exec(ctx, `UPDATE webhook_events SET status=$2, reason=$3 WHERE event_id=$1`, eventID, status, reason)
	return err
}

// TakeParked removes and returns the events parked for a subscription, to be delivered again.
func (s *Store) TakeParked(ctx context.Context, subRef string) ([]models.Event, error) {
	rows, err := s.db.Query(ctx, `DELETE FROM webhook_events WHERE status='parked' AND provider_sub_ref=$1 RETURNING payload`, subRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Event
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var ev models.Event
		if json.Unmarshal(raw, &ev) == nil {
			out = append(out, ev)
		}
	}
	return out, rows.Err()
}

func (s *Store) Webhooks(ctx context.Context, limit int) ([]models.WebhookRow, error) {
	rows, err := s.db.Query(ctx, `SELECT received_at, event_id, type, provider_sub_ref, status, reason FROM webhook_events
		ORDER BY received_at DESC, event_id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.WebhookRow
	for rows.Next() {
		var v models.WebhookRow
		if err := rows.Scan(&v.At, &v.ID, &v.Type, &v.SubRef, &v.Status, &v.Reason); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
