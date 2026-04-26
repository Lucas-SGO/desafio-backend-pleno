package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	otelgin "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"github.com/lucaseray/desafio-backend-pleno/internal/config"
	"github.com/lucaseray/desafio-backend-pleno/internal/db"
	"github.com/lucaseray/desafio-backend-pleno/internal/dlq"
	"github.com/lucaseray/desafio-backend-pleno/internal/middleware"
	"github.com/lucaseray/desafio-backend-pleno/internal/notification"
	redisclient "github.com/lucaseray/desafio-backend-pleno/internal/redis"
	"github.com/lucaseray/desafio-backend-pleno/internal/telemetry"
	"github.com/lucaseray/desafio-backend-pleno/internal/webhook"
	"github.com/lucaseray/desafio-backend-pleno/internal/ws"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	shutdown, err := telemetry.Init(ctx, "notificacoes", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("otel init: %v", err)
	}
	defer shutdown(ctx)

	database, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		log.Fatalf("run migrations: %v", err)
	}
	log.Println("migrations applied")

	rdb, err := redisclient.New(cfg.RedisURL)
	if err != nil {
		log.Fatalf("connect to redis: %v", err)
	}
	log.Println("redis connected")

	repo := notification.NewBreakeredRepository(notification.NewRepository(database))

	// Wire service with nil DLQ first, then set worker after both are created.
	svc := notification.NewService(repo, rdb, nil)
	dlqWorker := dlq.NewWorker(rdb, svc.DLQProcessFunc())
	svc.SetDLQWorker(dlqWorker)
	go dlqWorker.Run(ctx)

	hub := ws.NewHub(rdb)
	go hub.Run(ctx)

	router := gin.New()
	router.Use(gin.Recovery(), otelgin.Middleware("notificacoes"))
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	webhookGroup := router.Group("/webhook", middleware.WebhookSignature(cfg.WebhookSecret))
	webhook.NewHandler(svc, cfg.CPFHMACSecret).Register(webhookGroup)

	jwtMW := middleware.BearerJWT(cfg.JWTSecret, cfg.CPFHMACSecret)

	notifGroup := router.Group("/notifications", jwtMW)
	notification.NewHandler(repo).Register(notifGroup)

	wsGroup := router.Group("/ws", jwtMW)
	ws.NewHandler(hub).Register(wsGroup)

	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server: %v", err)
	}
}
