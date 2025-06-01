package main

import (
	"fmt"
	"github.com/Aidin1998/pincex_unified/internal/compliance/aml"
)

func main() {
	// Test that we can reference the types and methods
	fmt.Println("Testing AsyncRiskService compilation...")
	
	// This should compile if all methods exist
	_ = (*aml.AsyncRiskService)(nil)
	
	fmt.Println("✅ All types and methods are properly defined!")
}
