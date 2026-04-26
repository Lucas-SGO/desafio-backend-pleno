package dlq

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/lucaseray/desafio-backend-pleno/internal/domain"
)

const (
	queueKey    = "dlq:webhook:events"
	deadKey     = "dlq:webhook:dead"
	maxRetries  = 3
	popTimeout  = 5 * time.Second
)

// Entry is the DLQ envelope stored in Redis.
type Entry struct {
	Payload    domain.WebhookPayload `json:"payload"`
	CPFHash    string                `json:"cpf_hash"`
	EventHash  string                `json:"event_hash"`
	Retries    int                   `json:"retries"`
	EnqueuedAt time.Time             `json:"enqueued_at"`
	LastError  string                `json:"last_error"`
}

// ProcessFunc is the function the worker calls to reprocess an entry.
// It should return nil on success, ErrDuplicate to discard, or any other
// error to trigger a retry.
type ProcessFunc func(ctx context.Context, e Entry) error

// ErrDuplicate signals that the event was already processed — discard it.
var ErrDuplicate = errors.New("duplicate")

// Worker reads entries from the DLQ and retries them using fn.
type Worker struct {
	rdb *redis.Client
	fn  ProcessFunc
}

func NewWorker(rdb *redis.Client, fn ProcessFunc) *Worker {
	return &Worker{rdb: rdb, fn: fn}
}

// Enqueue pushes an entry to the front of the DLQ queue.
func (w *Worker) Enqueue(ctx context.Context, e Entry) error {
	e.EnqueuedAt = time.Now().UTC()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return w.rdb.LPush(ctx, queueKey, b).Err()
}

// Run blocks, continuously popping and reprocessing entries.
// Call it in a goroutine: go worker.Run(ctx).
func (w *Worker) Run(ctx context.Context) {
	log.Println("dlq: worker started")
	for {
		select {
		case <-ctx.Done():
			log.Println("dlq: worker stopped")
			return
		default:
		}

		result, err := w.rdb.BRPop(ctx, popTimeout, queueKey).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue // timeout, queue empty
			}
			if ctx.Err() != nil {
				return
			}
			log.Printf("dlq: brpop error: %v", err)
			continue
		}

		var entry Entry
		if err := json.Unmarshal([]byte(result[1]), &entry); err != nil {
			log.Printf("dlq: invalid entry, discarding: %v", err)
			continue
		}

		w.process(ctx, entry)
	}
}

func (w *Worker) process(ctx context.Context, entry Entry) {
	err := w.fn(ctx, entry)
	if err == nil || errors.Is(err, ErrDuplicate) {
		log.Printf("dlq: reprocessed chamado_id=%s retries=%d", entry.Payload.ChamadoID, entry.Retries)
		return
	}

	entry.LastError = err.Error()
	entry.Retries++

	if entry.Retries >= maxRetries {
		w.moveToDead(ctx, entry)
		return
	}

	// Exponential backoff: 2^retries seconds before re-enqueue.
	backoff := time.Duration(1<<entry.Retries) * time.Second
	log.Printf("dlq: retry %d/%d for chamado_id=%s in %s: %v",
		entry.Retries, maxRetries, entry.Payload.ChamadoID, backoff, err)

	select {
	case <-ctx.Done():
		return
	case <-time.After(backoff):
	}

	if enqErr := w.Enqueue(ctx, entry); enqErr != nil {
		log.Printf("dlq: failed to re-enqueue: %v", enqErr)
	}
}

func (w *Worker) moveToDead(ctx context.Context, entry Entry) {
	log.Printf("dlq: moving chamado_id=%s to dead letter after %d retries: %s",
		entry.Payload.ChamadoID, entry.Retries, entry.LastError)
	b, _ := json.Marshal(entry)
	if err := w.rdb.LPush(ctx, deadKey, b).Err(); err != nil {
		log.Printf("dlq: failed to write to dead letter: %v", err)
	}
}
