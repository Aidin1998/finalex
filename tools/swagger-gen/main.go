package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// OpenAPISpec represents the complete OpenAPI specification
type OpenAPISpec struct {
	OpenAPI    string                   `yaml:"openapi" json:"openapi"`
	Info       map[string]interface{}   `yaml:"info" json:"info"`
	Servers    []map[string]interface{} `yaml:"servers" json:"servers"`
	Paths      map[string]interface{}   `yaml:"paths" json:"paths"`
	Components map[string]interface{}   `yaml:"components" json:"components"`
	Security   []map[string]interface{} `yaml:"security" json:"security"`
	Tags       []map[string]interface{} `yaml:"tags" json:"tags"`
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run convert.go <output-dir>")
	}

	outputDir := os.Args[1]

	// Read the main OpenAPI YAML file
	yamlPath := "docs/api/openapi.yaml"
	yamlData, err := ioutil.ReadFile(yamlPath)
	if err != nil {
		log.Fatalf("Failed to read OpenAPI YAML: %v", err)
	}

	// Parse YAML
	var spec OpenAPISpec
	err = yaml.Unmarshal(yamlData, &spec)
	if err != nil {
		log.Fatalf("Failed to parse OpenAPI YAML: %v", err)
	}

	// Ensure output directory exists
	err = os.MkdirAll(outputDir, 0755)
	if err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Convert to JSON and write swagger.json
	jsonData, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal JSON: %v", err)
	}

	jsonPath := filepath.Join(outputDir, "swagger.json")
	err = ioutil.WriteFile(jsonPath, jsonData, 0644)
	if err != nil {
		log.Fatalf("Failed to write JSON file: %v", err)
	}

	// Create docs.go file
	docsContent := fmt.Sprintf(`package swagger

const SwaggerInfo = %q
`, string(jsonData))

	docsPath := filepath.Join(outputDir, "docs.go")
	err = ioutil.WriteFile(docsPath, []byte(docsContent), 0644)
	if err != nil {
		log.Fatalf("Failed to write docs.go file: %v", err)
	}

	fmt.Printf("Successfully generated Swagger documentation:\n")
	fmt.Printf("- JSON: %s\n", jsonPath)
	fmt.Printf("- Go: %s\n", docsPath)
}
