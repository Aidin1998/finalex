package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/hashicorp/consul/api"
	"github.com/spf13/viper"

	"services/marketfeeds/internal/connector"
)

func initConfig() {
	viper.SetDefault("CONSUL_ADDR", "localhost:8500")
	viper.SetDefault("KAFKA_ADDR", "localhost:9092")
	viper.SetDefault("SERVICE_PORT", "8080")
	viper.AutomaticEnv()
}

func main() {
	// Load configuration
	initConfig()
	exchange := os.Getenv("EXCHANGE_NAME")
	if exchange == "" {
		log.Fatal("EXCHANGE_NAME env var required")
	}
	consulAddr := viper.GetString("CONSUL_ADDR")
	servicePort := viper.GetString("SERVICE_PORT")

	// Consul registration
	consulCfg := api.DefaultConfig()
	consulCfg.Address = consulAddr
	client, err := api.NewClient(consulCfg)
	if err != nil {
		log.Fatalf("Consul client error: %v", err)
	}
	portInt, _ := strconv.Atoi(servicePort)
	host := os.Getenv("HOST_IP") // should be set via K8s downward API
	registration := &api.AgentServiceRegistration{
		Name:    "connector-" + exchange,
		ID:      fmt.Sprintf("connector-%s-%s", exchange, host),
		Tags:    []string{exchange},
		Address: host,
		Port:    portInt,
	}
	if err := client.Agent().ServiceRegister(registration); err != nil {
		log.Fatalf("Consul register error: %v", err)
	}

	// Initialize connector from registry
	factory := connector.Factory(exchange)
	if factory == nil {
		log.Fatalf("No connector factory for exchange %s", exchange)
	}
	conn := factory()
	cfg := connector.ExchangeConfig{
		WSURL:      viper.GetString("WSURL"),
		APIKey:     viper.GetString("API_KEY"),
		Secret:     viper.GetString("SECRET_KEY"),
		Passphrase: viper.GetString("PASSPHRASE"),
	}
	if err := conn.Init(cfg); err != nil {
		log.Fatalf("Connector init error: %v", err)
	}

	// Run connector
	ctx := context.Background()
	conn.Run(ctx)
}
