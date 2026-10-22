package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// checkTimeout bounds a background check (Android lets a worker run ~10 min).
const checkTimeout = 8 * time.Minute

// NewSeries is a series that got new chapters in a background check.
type newSeries struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	LatestChapter *float64 `json:"latest_chapter"`
}

// CheckNow runs one periodic check (the series that are due) for the
// Android background worker and returns, as JSON, the series that got new
// chapters in it: [{"id", "title", "latest_chapter"}]. It starts the server
// if needed, and the server stops afterwards unless the app holds it.
func CheckNow(dataDir string) (string, error) {
	mu.Lock()
	base, err := startLocked(dataDir)
	if err != nil {
		mu.Unlock()
		return "", err
	}
	checkHolds++
	s := sched
	mu.Unlock()
	defer func() {
		mu.Lock()
		checkHolds--
		stopLocked()
	}()
	if s == nil {
		return "", errors.New("check: no scheduler")
	}
	ctx, stop := context.WithTimeout(context.Background(), checkTimeout)
	defer stop()
	start := time.Now()
	s.CheckDue(ctx)
	found, err := newSince(ctx, base, start)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(found)
	if err != nil {
		return "", fmt.Errorf("encode check result: %w", err)
	}
	return string(data), nil
}

// newSince lists the series whose newest chapter was found after t.
func newSince(ctx context.Context, base string, t time.Time) ([]newSeries, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"api/v1/series?has_new=true", nil)
	if err != nil {
		return nil, fmt.Errorf("list new series: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list new series: %w", err)
	}
	defer func() { _ = resp.Body.Close() }() // read-only
	var body struct {
		Series []struct {
			newSeries
			LastNewChapterAt *time.Time `json:"last_new_chapter_at"`
		} `json:"series"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("list new series: %w", err)
	}
	out := []newSeries{}
	for _, sr := range body.Series {
		if sr.LastNewChapterAt != nil && !sr.LastNewChapterAt.Before(t) {
			out = append(out, sr.newSeries)
		}
	}
	return out, nil
}
