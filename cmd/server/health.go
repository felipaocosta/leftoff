package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/felipaocosta/leftoff/internal/app"
)

// healthCommand checks the server on localhost:PORT, for container
// healthchecks in images without curl (distroless).
func healthCommand(ctx context.Context, cfg app.Config) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	url := "http://127.0.0.1:" + strconv.Itoa(cfg.Port) + "/api/v1/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("health: %w", err)
	}
	defer func() { _ = resp.Body.Close() }() // read-only probe
	var h struct{ Status string }
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil || h.Status != "ok" {
		return fmt.Errorf("health: unexpected answer (HTTP %d)", resp.StatusCode)
	}
	return nil
}
