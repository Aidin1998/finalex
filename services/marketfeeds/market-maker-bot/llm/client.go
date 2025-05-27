package llm

import (
	"bytes"
	"encoding/json"
	"net/http"
)

func NewDefaultClient() *Client {
	return &Client{
		httpClient: &http.Client{},
	}
}

// LLMRequest is the schema for sending data to the LLM API.
type LLMRequest struct {
	Input string `json:"input"`
}

// LLMResponse is the schema for receiving data from the LLM API.
type LLMResponse struct {
	Output string `json:"output"`
}

// Client is a REST client for the LLM API.
type Client struct {
	APIURL     string
	httpClient *http.Client
}

// NewClient creates a new LLM API client.
func NewClient(apiURL string) *Client {
	return &Client{APIURL: apiURL}
}

// Predict sends a request to the LLM API and returns the prediction output.
func (c *Client) Predict(input string) (string, error) {
	body, err := json.Marshal(LLMRequest{Input: input})
	if err != nil {
		return "", err
	}
	resp, err := http.Post(c.APIURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", err
	}
	var llmResp LLMResponse
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return "", err
	}
	return llmResp.Output, nil
}
