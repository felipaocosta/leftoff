package app

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// LocalFlareSolverr is where a FlareSolverr container listens by default.
const LocalFlareSolverr = "http://127.0.0.1:8191"

// DetectFlareSolverr returns base if a FlareSolverr answers there, so the
// desktop app uses a local one without configuration (some sites, such as
// comix.to, reject any browser controlled through DevTools).
func DetectFlareSolverr(ctx context.Context, base string) string {
	ctx, cancel := context.WithTimeout(ctx, 700*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/", nil)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }() // read-only probe
	var body struct{ Msg string }
	if json.NewDecoder(resp.Body).Decode(&body) != nil || body.Msg == "" {
		return ""
	}
	return base
}
