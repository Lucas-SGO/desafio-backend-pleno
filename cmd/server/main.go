package main

import (
	"log"

	"github.com/lucaseray/desafio-backend-pleno/internal/config"
	"github.com/lucaseray/desafio-backend-pleno/internal/db"
	redisclient "github.com/lucaseray/desafio-backend-pleno/internal/redis"
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

	_, err = redisclient.New(cfg.RedisURL)
	if err != nil {
		log.Fatalf("connect to redis: %v", err)
	}
	log.Println("redis connected")

	log.Printf("server starting on port %s", cfg.Port)
}
