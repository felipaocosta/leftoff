// Package llm asks a local language model (Ollama, LM Studio or any server
// with the OpenAI-compatible /v1/chat/completions API) to write a recipe
// for a site no adapter knows (RF-54, SPEC §5.5). Its output is untrusted:
// the recipe is parsed, validated and tested against the page, and the
// model gets one more try with the problems found.
//
// It does NOT save recipes nor touch the library: the caller shows the
// recipe to the user, who saves it (RULES §6.8). It is never used by the
// periodic checks.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxResponseBytes limits the model server's answer.
const maxResponseBytes = 1 << 20

// ErrModel means the model server failed or gave an unusable answer.
var ErrModel = errors.New("language model error")

// Message is one chat message.
type Message struct {
	Role    string `json:"role"` // system, user or assistant
	Content string `json:"content"`
}

// Client talks to an OpenAI-compatible chat completions endpoint.
type Client struct {
	endpoint string
	model    string
	hc       *http.Client
}

// NewClient returns a client for the server at baseURL (e.g.
// http://localhost:11434 for Ollama or http://localhost:1234 for LM
// Studio, with or without /v1) and the given model.
func NewClient(baseURL, model string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	base := strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1")
	return &Client{endpoint: base + "/v1/chat/completions", model: model, hc: hc}
}

// Model is the configured model name.
func (c *Client) Model() string { return c.model }

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	Temperature    float64         `json:"temperature"`
	Stream         bool            `json:"stream"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type       string     `json:"type"`
	JSONSchema jsonSchema `json:"json_schema"`
}

type jsonSchema struct {
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	// Error is {"message": ...} (OpenAI, Ollama) or a plain string.
	Error json.RawMessage `json:"error"`
}

// errorText reads both error shapes.
func errorText(raw json.RawMessage) string {
	var obj struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Message != "" {
		return obj.Message
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

// Complete sends the conversation and returns the model's reply, which the
// server constrains to schema (a JSON Schema) when it supports it.
func (c *Client) Complete(ctx context.Context, msgs []Message, schema json.RawMessage) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.model, Messages: msgs, Temperature: 0.1,
		ResponseFormat: &responseFormat{
			Type:       "json_schema",
			JSONSchema: jsonSchema{Name: "recipe", Strict: true, Schema: schema},
		},
	})
	if err != nil {
		return "", fmt.Errorf("encode chat request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrModel, err)
	}
	defer func() { _ = resp.Body.Close() }() // read-only body
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("%w: read reply: %w", ErrModel, err)
	}
	var out chatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("%w: HTTP %d, reply is not JSON", ErrModel, resp.StatusCode)
	}
	switch {
	case len(out.Error) > 0 && string(out.Error) != "null":
		return "", fmt.Errorf("%w: %s", ErrModel, errorText(out.Error))
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("%w: HTTP %d", ErrModel, resp.StatusCode)
	case len(out.Choices) == 0:
		return "", fmt.Errorf("%w: empty reply", ErrModel)
	}
	return out.Choices[0].Message.Content, nil
}
