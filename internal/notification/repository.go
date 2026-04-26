package notification

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"
	"github.com/lucaseray/desafio-backend-pleno/internal/domain"
)

var ErrDuplicateEvent = errors.New("duplicate event")
var ErrNotFound = errors.New("notification not found")

type Repository interface {
	CreateFromEvent(ctx context.Context, cpfHash, eventHash string, p domain.WebhookPayload) (*domain.Notification, error)
	List(ctx context.Context, cpfHash string, limit, offset int) ([]domain.Notification, int, error)
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
