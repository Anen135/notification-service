package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"notification-service/internal/models"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*Repository, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Repository{pool: pool}, nil
}

func (r *Repository) Close() {
	r.pool.Close()
}

func (r *Repository) CreateNotification(ctx context.Context, n *models.Notification) error {
	meta, err := json.Marshal(n.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	const q = `
		INSERT INTO notifications
			(id, channel, recipient, subject, body, status, attempts, max_attempts, metadata, created_at, updated_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err = r.pool.Exec(ctx, q,
		n.ID, n.Channel, n.Recipient, n.Subject, n.Body, n.Status,
		n.Attempts, n.MaxAttempts, meta, n.CreatedAt, n.UpdatedAt,
	)
	return err
}

func (r *Repository) GetNotification(ctx context.Context, id uuid.UUID) (*models.Notification, error) {
	const q = `
		SELECT id, channel, recipient, subject, body, status, attempts, max_attempts,
		       metadata, error, created_at, updated_at, sent_at
		FROM notifications WHERE id = $1`

	row := r.pool.QueryRow(ctx, q, id)
	return scanNotification(row)
}

func (r *Repository) ListNotifications(ctx context.Context, status models.Status, limit, offset int) ([]*models.Notification, error) {
	var rows pgx.Rows
	var err error

	if status != "" {
		const q = `
			SELECT id, channel, recipient, subject, body, status, attempts, max_attempts,
			       metadata, error, created_at, updated_at, sent_at
			FROM notifications WHERE status = $1
			ORDER BY created_at DESC LIMIT $2 OFFSET $3`
		rows, err = r.pool.Query(ctx, q, status, limit, offset)
	} else {
		const q = `
			SELECT id, channel, recipient, subject, body, status, attempts, max_attempts,
			       metadata, error, created_at, updated_at, sent_at
			FROM notifications
			ORDER BY created_at DESC LIMIT $1 OFFSET $2`
		rows, err = r.pool.Query(ctx, q, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UpdateAfterAttempt persists the outcome of a single delivery attempt.
func (r *Repository) UpdateAfterAttempt(ctx context.Context, id uuid.UUID, status models.Status, attempts int, sendErr string) error {
	const q = `
		UPDATE notifications
		SET status = $2, attempts = $3, error = $4, updated_at = $5,
		    sent_at = CASE WHEN $2 = 'sent' THEN $5 ELSE sent_at END
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id, status, attempts, sendErr, time.Now().UTC())
	return err
}

func (r *Repository) CreateTemplate(ctx context.Context, t *models.Template) error {
	const q = `
		INSERT INTO templates (id, name, channel, subject, body, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.pool.Exec(ctx, q, t.ID, t.Name, t.Channel, t.Subject, t.Body, t.CreatedAt)
	return err
}

func (r *Repository) GetTemplateByName(ctx context.Context, name string) (*models.Template, error) {
	const q = `SELECT id, name, channel, subject, body, created_at FROM templates WHERE name = $1`
	row := r.pool.QueryRow(ctx, q, name)

	var t models.Template
	if err := row.Scan(&t.ID, &t.Name, &t.Channel, &t.Subject, &t.Body, &t.CreatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

// pgxRows is a narrow interface satisfied by both pgx.Rows and pgx.Row-like
// scanning so scanNotification can be shared between single-row and
// multi-row queries.
type pgxRows interface {
	Scan(dest ...any) error
}

func scanNotification(row pgxRows) (*models.Notification, error) {
	var n models.Notification
	var metaRaw []byte
	var sentAt *time.Time

	if err := row.Scan(
		&n.ID, &n.Channel, &n.Recipient, &n.Subject, &n.Body, &n.Status,
		&n.Attempts, &n.MaxAttempts, &metaRaw, &n.Error, &n.CreatedAt, &n.UpdatedAt, &sentAt,
	); err != nil {
		return nil, err
	}

	if len(metaRaw) > 0 {
		_ = json.Unmarshal(metaRaw, &n.Metadata)
	}
	n.SentAt = sentAt
	return &n, nil
}
