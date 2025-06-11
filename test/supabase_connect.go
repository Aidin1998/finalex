package main

import (
	"fmt"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	dsn := os.Getenv("SUPABASE_DSN")
	if dsn == "" {
		dsn = "postgresql://postgres.aiywrixoivhazskkisjz:Aidin!98@aws-0-ap-southeast-1.pooler.supabase.com:6543/postgres"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Printf("Failed to connect to Supabase: %v\n", err)
		os.Exit(1)
	}
	_ = db // prevent unused variable error
	fmt.Println("Successfully connected to Supabase!")
}
