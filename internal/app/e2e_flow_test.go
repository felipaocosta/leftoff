package app

import (
	"encoding/json"
	"strings"
	"testing"
)

const gfURL = "https://www.mgeko.cc/manga/grand-finale/"

// apiSeries is the part of a series the end-to-end tests read.
type apiSeries struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	LatestChapter   *float64 `json:"latest_chapter"`
	LastReadChapter *float64 `json:"last_read_chapter"`
	HasNew          bool     `json:"has_new"`
	UnreadCount     int      `json:"unread_count"`
	Notes           string   `json:"notes"`
	Cover           struct {
		Local string `json:"local"`
	} `json:"cover"`
	External *struct {
		Source string `json:"source"`
		ID     string `json:"id"`
	} `json:"external"`
	CheckError *struct {
		Message string `json:"message"`
	} `json:"check_error"`
	Chapters []struct {
		Key  string `json:"key"`
		URL  string `json:"url"`
		Read bool   `json:"read"`
	} `json:"chapters"`
}

func (s apiSeries) read() map[string]bool {
	m := map[string]bool{}
	for _, c := range s.Chapters {
		if c.Read {
			m[c.Key] = true
		}
	}
	return m
}

// grandFinale serves the mgeko series; without the newest chapter (50)
// until release is called, like a site before a release.
func grandFinale(t *testing.T, w *world) (release func()) {
	all := string(fixture(t, "mgeko.cc/grand-finale-all-chapters.html"))
	start := strings.Index(all, "<li data-chapterno")
	end := strings.Index(all[start:], "</li>") + start + len("</li>")
	w.set(gfURL, fixture(t, "mgeko.cc/grand-finale.html"))
	w.set(gfURL+"all-chapters/", []byte(all[:start]+all[end:]))
	return func() { w.set(gfURL+"all-chapters/", []byte(all)) }
}

func TestE2E_AddReadRefreshAndRestartKeepState(t *testing.T) {
	w := newWorld()
	release := grandFinale(t, w)
	cfg := Config{DataDir: t.TempDir()}
	srv := startServer(t, cfg, w, ServeOptions{Version: "e2e"})

	var preview struct {
		Method            string  `json:"method"`
		Confidence        float64 `json:"confidence"`
		NeedsConfirmation bool    `json:"needs_confirmation"`
		Chapters          []any   `json:"chapters"`
	}
	if code := srv.call("POST", "/extract/preview", map[string]string{"url": gfURL}, &preview); code != 200 ||
		preview.Method != "adapter:mgeko" || preview.NeedsConfirmation || len(preview.Chapters) != 49 {
		t.Fatalf("preview = %d %+v", code, preview)
	}

	var s apiSeries
	if code := srv.call("POST", "/series", map[string]any{"url": gfURL, "overrides": map[string]string{"title": "GF"}}, &s); code != 201 {
		t.Fatalf("add = %d", code)
	}
	if s.Title != "GF" || *s.LatestChapter != 49 || s.UnreadCount != 49 || s.Cover.Local == "" {
		t.Errorf("added = %+v", s)
	}
	var dup apiError
	if code := srv.call("POST", "/series", map[string]any{"url": gfURL + "?utm_source=x"}, &dup); code != 409 || dup.Error.Code != "duplicate" {
		t.Errorf("duplicate add = %d %s", code, dup.Error.Code)
	}

	if code := srv.call("POST", "/series/"+s.ID+"/read", map[string]any{"up_to": 10}, &s); code != 200 || *s.LastReadChapter != 10 {
		t.Fatalf("read up to = %d %v", code, s.LastReadChapter)
	}
	if code := srv.call("PATCH", "/series/"+s.ID, map[string]any{"notes": "lendo no ônibus"}, &s); code != 200 {
		t.Fatalf("edit = %d", code)
	}

	release() // chapter 50 comes out
	var refreshed struct {
		NewChapters int       `json:"new_chapters"`
		Series      apiSeries `json:"series"`
	}
	if code := srv.call("POST", "/series/"+s.ID+"/refresh", nil, &refreshed); code != 200 || refreshed.NewChapters != 1 {
		t.Fatalf("refresh = %d %+v; requests %v", code, refreshed.NewChapters, w.requests)
	}
	r := refreshed.Series
	if *r.LatestChapter != 50 || !r.HasNew || len(r.read()) != 10 || r.Title != "GF" || r.Notes != "lendo no ônibus" {
		t.Errorf("after refresh = latest %v new %v read %d %q %q", *r.LatestChapter, r.HasNew, len(r.read()), r.Title, r.Notes)
	}
	var list struct {
		Series []apiSeries `json:"series"`
	}
	if srv.call("GET", "/series?has_new=true", nil, &list); len(list.Series) != 1 || list.Series[0].Chapters != nil {
		t.Errorf("has_new list = %+v", list.Series)
	}
	srv.call("POST", "/series/"+s.ID+"/seen", nil, &s)
	if s.HasNew {
		t.Error("seen did not clear has_new")
	}
	if coverStatus, _ := srv.raw("GET", "/covers/"+s.ID, nil); coverStatus != 200 {
		t.Errorf("cover = %d", coverStatus)
	}

	// Restart on the same data dir: everything is still there.
	srv.stop()
	srv = startServer(t, cfg, w, ServeOptions{Version: "e2e"})
	var again apiSeries
	if code := srv.call("GET", "/series/"+s.ID, nil, &again); code != 200 || len(again.read()) != 10 ||
		*again.LatestChapter != 50 || again.Notes != "lendo no ônibus" || again.HasNew {
		t.Errorf("after restart = %d %+v", code, again)
	}

	// Export and import the same file: nothing is added or changed.
	_, export := srv.raw("GET", "/export", nil)
	var report struct {
		Added        int `json:"added"`
		Merged       int `json:"merged"`
		ChaptersRead int `json:"chapters_read"`
	}
	code, body := srv.raw("POST", "/import", strings.NewReader(export))
	if err := json.Unmarshal([]byte(body), &report); code != 200 || err != nil {
		t.Fatalf("import = %d %s", code, body)
	}
	if report.Added != 0 || report.Merged != 1 || report.ChaptersRead != 0 {
		t.Errorf("re-import = %+v", report)
	}

	if code := srv.call("DELETE", "/series/"+s.ID, nil, nil); code != 204 {
		t.Errorf("delete = %d", code)
	}
	if code := srv.call("GET", "/series/"+s.ID, nil, nil); code != 404 {
		t.Errorf("deleted series = %d", code)
	}
}
