package main

import (
	"log"

	"github.com/spf13/viper"
)

// Config holds service settings
type Config struct {
	WSURL                   string
	APIKey                  string
	Secret                  string
	Passphrase              string
	KafkaAddr               string
	HTTPPort                string
	ConsulAddr              string
	OutlierThresholdPercent float64
	MarketMakerRangePercent float64
}

func loadConfig() *Config {
	viper.SetDefault("WSURL", "wss://ws-feed.exchange.coinbase.com")
	viper.SetDefault("KafkaAddr", "localhost:9092")
	viper.SetDefault("HTTPPort", ":8080")
	viper.SetDefault("CONSUL_ADDR", "localhost:8500")
	viper.SetDefault("OUTLIER_THRESHOLD_PERCENT", 3)
	viper.SetDefault("MAKER_RANGE_PERCENT", 1)
	viper.SetDefault("EMERGENCY_PRICE", 0)
	viper.SetDefault("ADMIN_USER", "admin")
	viper.SetDefault("ADMIN_PASS", "password")
	viper.AutomaticEnv()

	cfg := &Config{
		WSURL:                   viper.GetString("WSURL"),
		APIKey:                  viper.GetString("API_KEY"),
		Secret:                  viper.GetString("SECRET_KEY"),
		Passphrase:              viper.GetString("PASSPHRASE"),
		KafkaAddr:               viper.GetString("KafkaAddr"),
		HTTPPort:                viper.GetString("HTTPPort"),
		ConsulAddr:              viper.GetString("CONSUL_ADDR"),
		OutlierThresholdPercent: viper.GetFloat64("OUTLIER_THRESHOLD_PERCENT"),
		MarketMakerRangePercent: viper.GetFloat64("MAKER_RANGE_PERCENT"),
	}
	log.Printf("Loaded config: %+v", cfg)
	return cfg
}
