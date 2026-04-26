package notification_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/lucaseray/desafio-backend-pleno/internal/db"
	"github.com/lucaseray/desafio-backend-pleno/internal/domain"
	"github.com/lucaseray/desafio-backend-pleno/internal/notification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDB creates an isolated schema for each test and returns a Repository.
// Requires TEST_DATABASE_URL env var; skips otherwise.
func setupTestDB(t *testing.T) notification.Repository {
	t.Helper()
	baseDSN := os.Getenv("TEST_DATABASE_URL")
	if baseDSN == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	// Open a control connection to create/drop the schema.
	control, err := sql.Open("postgres", baseDSN)
	require.NoError(t, err)
	require.NoError(t, control.Ping())

	schema := fmt.Sprintf("test_%d", time.Now().UnixNano())
	_, err = control.Exec(fmt.Sprintf(`CREATE SCHEMA "%s"`, schema))
	require.NoError(t, err)

	t.Cleanup(func() {
		control.Exec(fmt.Sprintf(`DROP SCHEMA "%s" CASCADE`, schema))
		control.Close()
	})

	// Open a separate pool with search_path pointing to the test schema.
	dsnWithSchema := addSearchPath(baseDSN, schema)
	database, err := db.Open(dsnWithSchema)
	require.NoError(t, err)

	require.NoError(t, db.RunMigrations(database))

	t.Cleanup(func() { database.Close() })

	return notification.NewRepository(database)
}

// addSearchPath appends search_path to a postgres DSN URL.
func addSearchPath(dsn, schema string) string {
	if strings.Contains(dsn, "?") {
		return dsn + "&search_path=" + schema
	}
	return dsn + "?search_path=" + schema
}

func makePayload(chamadoID string) domain.WebhookPayload {
	return domain.WebhookPayload{
		ChamadoID:      chamadoID,
		Tipo:           "status_change",
		CPF:            "12345678901",
		StatusAnterior: "aberto",
		StatusNovo:     "em_analise",
		Titulo:         "Buraco",
		Descricao:      "Rua X",
		Timestamp:      time.Now().UTC(),
	}
}

func TestRepository_CreateFromEvent(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	n, err := repo.CreateFromEvent(ctx, "hash123", "event-hash-1", makePayload("CH-001"))
	require.NoError(t, err)
	assert.NotEmpty(t, n.ID)
	assert.Equal(t, "CH-001", n.ChamadoID)
	assert.False(t, n.IsRead)
}

func TestRepository_DuplicateEvent(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	_, err := repo.CreateFromEvent(ctx, "hash123", "event-dup", makePayload("CH-002"))
	require.NoError(t, err)

	_, err = repo.CreateFromEvent(ctx, "hash123", "event-dup", makePayload("CH-002"))
	assert.ErrorIs(t, err, notification.ErrDuplicateEvent)
}

func TestRepository_List_Pagination(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := repo.CreateFromEvent(ctx, "hash-list", fmt.Sprintf("ev-%d", i), makePayload(fmt.Sprintf("CH-%d", i)))
		require.NoError(t, err)
	}

	items, total, err := repo.List(ctx, "hash-list", 2, 0)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, items, 2)
}

func TestRepository_MarkRead_Ownership(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	n, err := repo.CreateFromEvent(ctx, "owner-hash", "ev-own", makePayload("CH-own"))
	require.NoError(t, err)

	// wrong owner cannot mark as read
	err = repo.MarkRead(ctx, n.ID, "other-hash")
	assert.ErrorIs(t, err, notification.ErrNotFound)

	// correct owner succeeds
	err = repo.MarkRead(ctx, n.ID, "owner-hash")
	assert.NoError(t, err)
}

func TestRepository_UnreadCount(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := repo.CreateFromEvent(ctx, "hash-unread", fmt.Sprintf("ev-u%d", i), makePayload(fmt.Sprintf("CH-u%d", i)))
		require.NoError(t, err)
	}

	count, err := repo.UnreadCount(ctx, "hash-unread")
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}
