package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	apiURL  = "http://localhost:5000/api" // Update with the actual LLM API URL
	timeout = 10 * time.Second
)

// Removed duplicate LLMRequest declaration. It is now imported from client.go.

// Removed duplicate LLMResponse declaration. It is now imported from client.go.

// Removed duplicate Client declaration. It is now imported from client.go.

// Removed duplicate NewClient function. It is now imported from client.go.

func (c *Client) SendRequest(input string) (*LLMResponse, error) {
	reqBody := LLMRequest{Input: input}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get response: %s", resp.Status)
	}

	var llmResp LLMResponse
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return nil, err
	}

	return &llmResp, nil
}
