package app

import (
	"strings"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/library"
)

// The periodic check finds the new chapter by itself and sends one ntfy
// message for it, once (RF-22, RF-26).
func TestE2E_ScheduledCheckNotifiesOnce(t *testing.T) {
	w := newWorld()
	release := grandFinale(t, w)
	cfg := Config{
		DataDir: t.TempDir(), CheckInterval: 400 * time.Millisecond,
		Notify: library.NotifyEnv{NtfyURL: "https://ntfy.test/manga"},
	}
	srv := startServer(t, cfg, w, ServeOptions{})
	var s apiSeries
	if code := srv.call("POST", "/series", map[string]any{"url": gfURL}, &s); code != 201 {
		t.Fatalf("add = %d", code)
	}
	srv.call("POST", "/series/"+s.ID+"/read", map[string]any{"up_to": 49}, &s)
	release()

	var msg string
	select {
	case b := <-w.ntfy:
		msg = string(b)
	case <-time.After(10 * time.Second):
		t.Fatal("no notification after a release")
	}
	if !strings.Contains(msg, "Grand Finale") || !strings.Contains(msg, "50") ||
		!strings.Contains(msg, "/reader/en/grand-finale-chapter-50-eng-li/") {
		t.Errorf("notification = %s", msg)
	}
	srv.call("GET", "/series/"+s.ID, nil, &s)
	if len(s.read()) != 49 || !s.HasNew || s.UnreadCount != 1 {
		t.Errorf("after the check: read %d, has_new %v, unread %d", len(s.read()), s.HasNew, s.UnreadCount)
	}
	select { // more checks run, but chapter 50 was announced already
	case b := <-w.ntfy:
		t.Errorf("second notification: %s", b)
	case <-time.After(1500 * time.Millisecond):
	}
}

// "Refresh all" runs as a job; manual refreshes never notify (SPEC §9.1).
func TestE2E_RefreshAllJob(t *testing.T) {
	w := newWorld()
	release := grandFinale(t, w)
	cfg := Config{DataDir: t.TempDir(), Notify: library.NotifyEnv{NtfyURL: "https://ntfy.test/manga"}}
	srv := startServer(t, cfg, w, ServeOptions{})
	var s apiSeries
	srv.call("POST", "/series", map[string]any{"url": gfURL}, &s)
	release()

	var job struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		Total       int    `json:"total"`
		Done        int    `json:"done"`
		Failed      int    `json:"failed"`
		NewChapters int    `json:"new_chapters"`
	}
	if code := srv.call("POST", "/refresh", nil, &job); code != 202 || job.ID == "" {
		t.Fatalf("refresh all = %d %+v", code, job)
	}
	eventually(t, 10*time.Second, "the job to finish", func() bool {
		srv.call("GET", "/jobs/"+job.ID, nil, &job)
		return job.Status != "running"
	})
	if job.Total != 1 || job.Done != 1 || job.Failed != 0 || job.NewChapters != 1 {
		t.Errorf("job = %+v", job)
	}
	select {
	case b := <-w.ntfy:
		t.Errorf("a manual refresh notified: %s", b)
	case <-time.After(300 * time.Millisecond):
	}
}

// With enrichment on, adding a series links it to AniList (RF-57), and a
// failing site only records a check error.
func TestE2E_EnrichmentAndCheckErrors(t *testing.T) {
	w := newWorld()
	grandFinale(t, w)
	w.set("POST https://graphql.anilist.co", fixture(t, "anilist.co/search-grand-finale.json"))
	on := true
	srv := startServer(t, Config{DataDir: t.TempDir(), EnrichEnabled: &on}, w, ServeOptions{})
	var s apiSeries
	if code := srv.call("POST", "/series", map[string]any{"url": gfURL}, &s); code != 201 {
		t.Fatalf("add = %d", code)
	}
	eventually(t, 5*time.Second, "the AniList link", func() bool {
		srv.call("GET", "/series/"+s.ID, nil, &s)
		return s.External != nil && s.External.ID != ""
	})
	if s.External.Source != "anilist" || s.External.ID != "191787" {
		t.Errorf("external = %+v", s.External)
	}

	w.set(gfURL+"all-chapters/", []byte("<html><body>maintenance</body></html>"))
	var e apiError
	if code := srv.call("POST", "/series/"+s.ID+"/refresh", nil, &e); code < 400 {
		t.Fatalf("refresh of a broken page = %d", code)
	}
	srv.call("GET", "/series/"+s.ID, nil, &s)
	if s.CheckError == nil || len(s.Chapters) != 49 {
		t.Errorf("check_error = %+v, chapters = %d", s.CheckError, len(s.Chapters))
	}
}

// A site that only builds its page with JavaScript, on a device without a
// browser (Android): the refresh keeps every chapter and records
// needs_browser, which the UI explains for the device.
func TestE2E_JavaScriptShellWithoutABrowser(t *testing.T) {
	const vortex = "https://vortexscans.org/series/survival-supremacy"
	w := newWorld()
	w.set(vortex, fixture(t, "vortexscans.org/survival-supremacy.html"))
	srv := startServer(t, Config{DataDir: t.TempDir()}, w, ServeOptions{})
	var s apiSeries
	if code := srv.call("POST", "/series", map[string]any{"url": vortex}, &s); code != 201 {
		t.Fatalf("add = %d", code)
	}
	srv.call("POST", "/series/"+s.ID+"/read", map[string]any{"up_to": 5}, &s)

	w.set(vortex, []byte(`<html><head><title>Vortex Scans</title></head><body><div id="__next"></div></body></html>`))
	var e apiError
	if code := srv.call("POST", "/series/"+s.ID+"/refresh", nil, &e); code != 422 || e.Error.Code != "needs_browser" {
		t.Errorf("refresh = %d %s", code, e.Error.Code)
	}
	var got struct {
		apiSeries
		CheckError *struct {
			Code string `json:"code"`
		} `json:"check_error"`
	}
	srv.call("GET", "/series/"+s.ID, nil, &got)
	if got.CheckError == nil || got.CheckError.Code != "needs_browser" || len(got.Chapters) != 16 || len(got.read()) != 5 {
		t.Errorf("after the refresh: check_error %+v, %d chapters, %d read", got.CheckError, len(got.Chapters), len(got.read()))
	}
}
