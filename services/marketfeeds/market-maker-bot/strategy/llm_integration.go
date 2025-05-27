package strategy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// LLMRequest represents the request structure for the LLM API.
type LLMRequest struct {
	Input string `json:"input"`
}

// LLMResponse represents the response structure from the LLM API.
type LLMResponse struct {
	Output string `json:"output"`
}

// SendLLMRequest sends a request to the Python-hosted LLM API and returns the response.
func SendLLMRequest(apiURL string, input string) (string, error) {
	requestBody, err := json.Marshal(LLMRequest{Input: input})
	if err != nil {
		return "", err
	}

	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get response from LLM API: %s", resp.Status)
	}

	var llmResponse LLMResponse
	if err := json.NewDecoder(resp.Body).Decode(&llmResponse); err != nil {
		return "", err
	}

	return llmResponse.Output, nil
}
