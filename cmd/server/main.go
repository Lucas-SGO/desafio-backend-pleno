package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/lucaseray/desafio-backend-pleno/internal/config"
	"github.com/lucaseray/desafio-backend-pleno/internal/db"
	"github.com/lucaseray/desafio-backend-pleno/internal/middleware"
	"github.com/lucaseray/desafio-backend-pleno/internal/notification"
	redisclient "github.com/lucaseray/desafio-backend-pleno/internal/redis"
	"github.com/lucaseray/desafio-backend-pleno/internal/webhook"
)

func main() {
	cfg := config.Load()

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

	repo := notification.NewRepository(database)
	svc := notification.NewService(repo, rdb)

	router := gin.Default()

	webhookGroup := router.Group("/webhook", middleware.WebhookSignature(cfg.WebhookSecret))
	webhook.NewHandler(svc, cfg.CPFHMACSecret).Register(webhookGroup)

	jwtMW := middleware.BearerJWT(cfg.JWTSecret, cfg.CPFHMACSecret)
	notifGroup := router.Group("/notifications", jwtMW)
	notification.NewHandler(repo).Register(notifGroup)

	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server: %v", err)
	}
}
