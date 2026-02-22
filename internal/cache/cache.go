package cache

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
	"github.com/sentinel-app/sentinel/internal/config"
)

// Connect creates a Redis client and verifies connectivity.
func Connect(cfg *config.Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	log.Println("Redis connected successfully")
	return rdb
}
