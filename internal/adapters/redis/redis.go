package redis

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
)

func NewClient(addrOrURL string) *redis.Client {
	opts, err := redis.ParseURL(addrOrURL)
	if err != nil {
		// Not a URL — treat as a plain address (local dev / tests).
		opts = &redis.Options{Addr: addrOrURL}
	}

	rdb := redis.NewClient(opts)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Printf("Connected to Redis at %s", opts.Addr)
	return rdb
}
