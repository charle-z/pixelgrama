package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type ModerationStatus string

const (
	ModerationVisible ModerationStatus = "visible"
	ModerationHidden  ModerationStatus = "hidden"
	ModerationAll     ModerationStatus = "all"

	MaxModerationReasonRunes = 256
	MaxModerationListLimit   = 200
)

var (
	ErrPostcardNotFound        = errors.New("postcard not found")
	ErrModerationState         = errors.New("postcard moderation state does not allow this action")
	ErrInvalidModerationReason = errors.New("invalid moderation reason")
)

type ModerationRecord struct {
	ID          int64            `json:"id"`
	Alias       *string          `json:"alias,omitempty"`
	Commit      string           `json:"commit"`
	CreatedAt   time.Time        `json:"created_at"`
	Status      ModerationStatus `json:"status"`
	ModeratedAt *time.Time       `json:"moderated_at,omitempty"`
	Reason      *string          `json:"reason,omitempty"`
}

func (s *Store) ListModeration(ctx context.Context, status ModerationStatus, limit int) ([]ModerationRecord, error) {
	if status != ModerationAll && status != ModerationVisible && status != ModerationHidden {
		return nil, fmt.Errorf("invalid moderation status %q", status)
	}
	if limit < 1 || limit > MaxModerationListLimit {
		return nil, fmt.Errorf("moderation limit must be between 1 and %d", MaxModerationListLimit)
	}

	query := `SELECT id, alias, deployed_commit, created_at, moderation_status, moderated_at, moderation_reason
FROM postcards`
	args := make([]any, 0, 2)
	if status != ModerationAll {
		query += " WHERE moderation_status = ?"
		args = append(args, string(status))
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list moderation records: %w", err)
	}
	defer rows.Close()

	records := make([]ModerationRecord, 0, limit)
	for rows.Next() {
		record, err := scanModerationRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate moderation records: %w", err)
	}
	return records, nil
}

func (s *Store) Hide(ctx context.Context, id int64, reason string, at time.Time) (ModerationRecord, error) {
	return s.setModeration(ctx, id, ModerationVisible, ModerationHidden, "hide", reason, at)
}

func (s *Store) Restore(ctx context.Context, id int64, reason string, at time.Time) (ModerationRecord, error) {
	return s.setModeration(ctx, id, ModerationHidden, ModerationVisible, "restore", reason, at)
}

func (s *Store) setModeration(
	ctx context.Context,
	id int64,
	from ModerationStatus,
	to ModerationStatus,
	action string,
	reason string,
	at time.Time,
) (ModerationRecord, error) {
	if id < 1 {
		return ModerationRecord{}, ErrPostcardNotFound
	}
	normalizedReason, err := normalizeModerationReason(reason)
	if err != nil {
		return ModerationRecord{}, err
	}
	at = at.UTC()
	atText := at.Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ModerationRecord{}, fmt.Errorf("begin moderation action: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
UPDATE postcards
SET moderation_status = ?, moderated_at = ?, moderation_reason = ?
WHERE id = ? AND moderation_status = ?`, string(to), atText, normalizedReason, id, string(from))
	if err != nil {
		return ModerationRecord{}, fmt.Errorf("update postcard moderation: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return ModerationRecord{}, fmt.Errorf("read moderation update result: %w", err)
	}
	if changed != 1 {
		var current string
		err := tx.QueryRowContext(ctx, "SELECT moderation_status FROM postcards WHERE id = ?", id).Scan(&current)
		if errors.Is(err, sql.ErrNoRows) {
			return ModerationRecord{}, ErrPostcardNotFound
		}
		if err != nil {
			return ModerationRecord{}, fmt.Errorf("read postcard moderation state: %w", err)
		}
		return ModerationRecord{}, fmt.Errorf("%w: current=%s required=%s", ErrModerationState, current, from)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO moderation_events (postcard_id, action, reason, created_at)
VALUES (?, ?, ?, ?)`, id, action, normalizedReason, atText); err != nil {
		return ModerationRecord{}, fmt.Errorf("record moderation event: %w", err)
	}

	record, err := queryModerationRecord(ctx, tx, id)
	if err != nil {
		return ModerationRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModerationRecord{}, fmt.Errorf("commit moderation action: %w", err)
	}
	return record, nil
}

func normalizeModerationReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || !utf8.ValidString(reason) || utf8.RuneCountInString(reason) > MaxModerationReasonRunes {
		return "", ErrInvalidModerationReason
	}
	for _, value := range reason {
		if unicode.IsControl(value) {
			return "", ErrInvalidModerationReason
		}
	}
	return reason, nil
}

func queryModerationRecord(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64) (ModerationRecord, error) {
	row := queryer.QueryRowContext(ctx, `
SELECT id, alias, deployed_commit, created_at, moderation_status, moderated_at, moderation_reason
FROM postcards WHERE id = ?`, id)
	record, err := scanModerationRecord(row)
	if err != nil {
		return ModerationRecord{}, err
	}
	return record, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanModerationRecord(row rowScanner) (ModerationRecord, error) {
	var record ModerationRecord
	var alias sql.NullString
	var createdAt string
	var status string
	var moderatedAt sql.NullString
	var reason sql.NullString
	if err := row.Scan(
		&record.ID,
		&alias,
		&record.Commit,
		&createdAt,
		&status,
		&moderatedAt,
		&reason,
	); err != nil {
		return ModerationRecord{}, fmt.Errorf("scan moderation record: %w", err)
	}
	if alias.Valid {
		value := alias.String
		record.Alias = &value
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ModerationRecord{}, fmt.Errorf("decode postcard time %d: %w", record.ID, err)
	}
	record.CreatedAt = parsedCreatedAt
	record.Status = ModerationStatus(status)
	if moderatedAt.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, moderatedAt.String)
		if err != nil {
			return ModerationRecord{}, fmt.Errorf("decode moderation time %d: %w", record.ID, err)
		}
		record.ModeratedAt = &parsed
	}
	if reason.Valid {
		value := reason.String
		record.Reason = &value
	}
	return record, nil
}
