package notification

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/lucaseray/desafio-backend-pleno/internal/domain"
)

type Service struct {
	repo  Repository
	redis *redis.Client
}

func NewService(repo Repository, redis *redis.Client) *Service {
	return &Service{repo: repo, redis: redis}
}

func (s *Service) CreateFromWebhook(ctx context.Context, cpfHash, eventHash string, p domain.WebhookPayload) (*domain.Notification, error) {
	n, err := s.repo.CreateFromEvent(ctx, cpfHash, eventHash, p)
	if err != nil {
		return nil, err
	}

	if s.redis != nil {
		payload, _ := json.Marshal(n)
		s.redis.Publish(ctx, fmt.Sprintf("notifications:%s", cpfHash), payload)
	}

	return n, nil
}
