package notification

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/sony/gobreaker"
	"github.com/lucaseray/desafio-backend-pleno/internal/domain"
)

var ErrDuplicateEvent = errors.New("duplicate event")
var ErrNotFound = errors.New("notification not found")

type Repository interface {
	CreateFromEvent(ctx context.Context, cpfHash, eventHash string, p domain.WebhookPayload) (*domain.Notification, error)
	List(ctx context.Context, cpfHash string, limit, offset int) ([]domain.Notification, int, error)
	// ListCursor returns up to limit items before the given cursor position.
	// cursor="" fetches the first page. Returns nextCursor and hasMore.
	ListCursor(ctx context.Context, cpfHash string, limit int, cursor string) ([]domain.Notification, string, bool, error)
	MarkRead(ctx context.Context, id, cpfHash string) error
	UnreadCount(ctx context.Context, cpfHash string) (int, error)
}

type postgresRepository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) CreateFromEvent(ctx context.Context, cpfHash, eventHash string, p domain.WebhookPayload) (*domain.Notification, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO event_log (event_hash) VALUES ($1)`, eventHash)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return nil, ErrDuplicateEvent
		}
		return nil, err
	}

	var n domain.Notification
	err = tx.QueryRowContext(ctx, `
		INSERT INTO notifications
			(cpf_hash, chamado_id, titulo, descricao, status_anterior, status_novo)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, chamado_id, titulo, descricao, status_anterior, status_novo, is_read, created_at`,
		cpfHash, p.ChamadoID, p.Titulo, p.Descricao, p.StatusAnterior, p.StatusNovo,
	).Scan(&n.ID, &n.ChamadoID, &n.Titulo, &n.Descricao, &n.StatusAnterior, &n.StatusNovo, &n.IsRead, &n.CreatedAt)
	if err != nil {
		return nil, err
	}

	return &n, tx.Commit()
}

func (r *postgresRepository) List(ctx context.Context, cpfHash string, limit, offset int) ([]domain.Notification, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE cpf_hash = $1`, cpfHash,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, chamado_id, titulo, descricao, status_anterior, status_novo, is_read, created_at
		FROM notifications
		WHERE cpf_hash = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		cpfHash, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var notifications []domain.Notification
	for rows.Next() {
		var n domain.Notification
		if err := rows.Scan(&n.ID, &n.ChamadoID, &n.Titulo, &n.Descricao, &n.StatusAnterior, &n.StatusNovo, &n.IsRead, &n.CreatedAt); err != nil {
			return nil, 0, err
		}
		notifications = append(notifications, n)
	}
	return notifications, total, rows.Err()
}

func (r *postgresRepository) ListCursor(ctx context.Context, cpfHash string, limit int, cursor string) ([]domain.Notification, string, bool, error) {
	var (
		rows *sql.Rows
		err  error
	)

	// Fetch limit+1 to detect whether there is a next page.
	fetch := limit + 1

	if cursor == "" {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, chamado_id, titulo, descricao, status_anterior, status_novo, is_read, created_at
			FROM notifications
			WHERE cpf_hash = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2`,
			cpfHash, fetch,
		)
	} else {
		cursorTime, cursorID, decErr := decodeCursor(cursor)
		if decErr != nil {
			return nil, "", false, decErr
		}
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, chamado_id, titulo, descricao, status_anterior, status_novo, is_read, created_at
			FROM notifications
			WHERE cpf_hash = $1
			  AND (created_at, id) < ($2::timestamptz, $3::uuid)
			ORDER BY created_at DESC, id DESC
			LIMIT $4`,
			cpfHash, cursorTime, cursorID, fetch,
		)
	}
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()

	var items []domain.Notification
	for rows.Next() {
		var n domain.Notification
		if err := rows.Scan(&n.ID, &n.ChamadoID, &n.Titulo, &n.Descricao, &n.StatusAnterior, &n.StatusNovo, &n.IsRead, &n.CreatedAt); err != nil {
			return nil, "", false, err
		}
		items = append(items, n)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, err
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	var nextCursor string
	if hasMore {
		last := items[len(items)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}

	return items, nextCursor, hasMore, nil
}

func (r *postgresRepository) MarkRead(ctx context.Context, id, cpfHash string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET is_read = TRUE WHERE id = $1 AND cpf_hash = $2`,
		id, cpfHash,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *postgresRepository) UnreadCount(ctx context.Context, cpfHash string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE cpf_hash = $1 AND is_read = FALSE`,
		cpfHash,
	).Scan(&count)
	return count, err
}

// breakeredRepository wraps a Repository with a circuit breaker.
// After 5 consecutive failures the breaker opens for 30s, rejecting calls
// immediately instead of waiting for connection timeouts.
type breakeredRepository struct {
	inner   Repository
	breaker *gobreaker.CircuitBreaker
}

// NewBreakeredRepository decorates inner with a circuit breaker. The underlying
// Repository interface is unchanged — callers need no modification.
func NewBreakeredRepository(inner Repository) Repository {
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "postgres",
		MaxRequests: 1,
		Interval:    30 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= 5
		},
	})
	return &breakeredRepository{inner: inner, breaker: cb}
}

func (b *breakeredRepository) CreateFromEvent(ctx context.Context, cpfHash, eventHash string, p domain.WebhookPayload) (*domain.Notification, error) {
	result, err := b.breaker.Execute(func() (any, error) {
		return b.inner.CreateFromEvent(ctx, cpfHash, eventHash, p)
	})
	if err != nil {
		return nil, err
	}
	return result.(*domain.Notification), nil
}

func (b *breakeredRepository) List(ctx context.Context, cpfHash string, limit, offset int) ([]domain.Notification, int, error) {
	type listResult struct {
		items []domain.Notification
		total int
	}
	result, err := b.breaker.Execute(func() (any, error) {
		items, total, err := b.inner.List(ctx, cpfHash, limit, offset)
		return listResult{items, total}, err
	})
	if err != nil {
		return nil, 0, err
	}
	r := result.(listResult)
	return r.items, r.total, nil
}

func (b *breakeredRepository) ListCursor(ctx context.Context, cpfHash string, limit int, cursor string) ([]domain.Notification, string, bool, error) {
	type cursorResult struct {
		items      []domain.Notification
		nextCursor string
		hasMore    bool
	}
	result, err := b.breaker.Execute(func() (any, error) {
		items, next, more, err := b.inner.ListCursor(ctx, cpfHash, limit, cursor)
		return cursorResult{items, next, more}, err
	})
	if err != nil {
		return nil, "", false, err
	}
	r := result.(cursorResult)
	return r.items, r.nextCursor, r.hasMore, nil
}

func (b *breakeredRepository) MarkRead(ctx context.Context, id, cpfHash string) error {
	_, err := b.breaker.Execute(func() (any, error) {
		return nil, b.inner.MarkRead(ctx, id, cpfHash)
	})
	return err
}

func (b *breakeredRepository) UnreadCount(ctx context.Context, cpfHash string) (int, error) {
	result, err := b.breaker.Execute(func() (any, error) {
		count, err := b.inner.UnreadCount(ctx, cpfHash)
		return count, err
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}
