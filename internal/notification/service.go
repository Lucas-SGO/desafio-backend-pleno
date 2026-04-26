package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/lucaseray/desafio-backend-pleno/internal/dlq"
	"github.com/lucaseray/desafio-backend-pleno/internal/domain"
)

type Service struct {
	repo      Repository
	redis     *redis.Client
	dlqWorker *dlq.Worker
}

func NewService(repo Repository, redis *redis.Client, dlqWorker *dlq.Worker) *Service {
	return &Service{repo: repo, redis: redis, dlqWorker: dlqWorker}
}

func (s *Service) CreateFromWebhook(ctx context.Context, cpfHash, eventHash string, p domain.WebhookPayload) (*domain.Notification, error) {
	n, err := s.repo.CreateFromEvent(ctx, cpfHash, eventHash, p)
	if err != nil {
		if errors.Is(err, ErrDuplicateEvent) {
			return nil, err
		}
		// Transient failure: enqueue for retry if DLQ is configured.
		if s.dlqWorker != nil {
			enqErr := s.dlqWorker.Enqueue(ctx, dlq.Entry{
				Payload:   p,
				CPFHash:   cpfHash,
				EventHash: eventHash,
			})
			if enqErr != nil {
				// Log but still return the original error.
				fmt.Printf("dlq: enqueue failed: %v\n", enqErr)
			}
		}
		return nil, err
	}

	if s.redis != nil {
		payload, _ := json.Marshal(n)
		s.redis.Publish(ctx, fmt.Sprintf("notifications:%s", cpfHash), payload)
	}

	return n, nil
}

// reprocessEntry is used by the DLQ worker to retry a failed event.
func (s *Service) reprocessEntry(ctx context.Context, e dlq.Entry) error {
	_, err := s.CreateFromWebhook(ctx, e.CPFHash, e.EventHash, e.Payload)
	if errors.Is(err, ErrDuplicateEvent) {
		return dlq.ErrDuplicate
	}
	return err
}

// DLQProcessFunc returns the function the DLQ worker should call.
func (s *Service) DLQProcessFunc() dlq.ProcessFunc {
	return s.reprocessEntry
}

// SetDLQWorker breaks the construction cycle: create Service with nil worker,
// create Worker with svc.DLQProcessFunc(), then call this to complete the wiring.
func (s *Service) SetDLQWorker(w *dlq.Worker) {
	s.dlqWorker = w
}
