package config

import (
	"log"

	"github.com/spf13/viper"
)

type Config struct {
	StripeKey    string
	DBUrl        string
	RateAPIKeys  string
	RateLimitRPS int
}

func LoadConfig() *Config {
	viper.SetConfigFile(".env")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("No config file found: %v", err)
	}

	return &Config{
		StripeKey:    viper.GetString("STRIPE_KEY"),
		DBUrl:        viper.GetString("DB_URL"),
		RateAPIKeys:  viper.GetString("RATE_API_KEYS"),
		RateLimitRPS: viper.GetInt("RATE_LIMIT_RPS"),
	}
}
