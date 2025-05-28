package main

import (
	"log"

	"services/marketfeeds/aggregator"
	"services/marketfeeds/api"
	"services/marketfeeds/publisher"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := loadConfig()

	// 1. Start publisher
	pub := publisher.NewKafkaPublisher(cfg.KafkaAddr)
	defer pub.Close()

	// 2. Start live websocket aggregator
	ag := aggregator.New(cfg.WSURL, cfg.APIKey, cfg.Secret, cfg.Passphrase, pub)
	go ag.Run()

	// 3. HTTP server for health and average-price endpoint
	r := gin.Default()
	api.RegisterRoutes(r)
	log.Fatal(r.Run(cfg.HTTPPort))
}
