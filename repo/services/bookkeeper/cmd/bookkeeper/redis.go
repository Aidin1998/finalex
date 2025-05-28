package main

import (
	"context"

	"github.com/redis/go-redis/v9"
)

func NewRedis(url string) *redis.Client {
	opts, err := redis.ParseURL(url)
	if err != nil {
		panic(err)
	}
	client := redis.NewClient(opts)

	if _, err := client.Ping(context.Background()).Result(); err != nil {
		panic("failed to connect to Redis: " + err.Error())
	}

	return client
}
