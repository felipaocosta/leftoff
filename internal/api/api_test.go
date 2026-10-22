package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/extract/adapters/liliana"
	"github.com/felipaocosta/leftoff/internal/extract/recipe"
	"github.com/felipaocosta/leftoff/internal/fetch"
	"github.com/felipaocosta/leftoff/internal/library"
	"github.com/felipaocosta/leftoff/internal/scheduler"
	"github.com/felipaocosta/leftoff/internal/storage"
)

const (
	seriesURL = "https://manhuaplus.org/manga/forging-an-industrial-empire-in-a-magical-world"
	coverURL  = "https://manhuaplus.org/uploads/covers/forging-an-industrial-empire-in-a-magical-world.jpg"
	token     = "s3cret"
)

// newTestAPI wires the real stack offline: storage in a temp dir, the
// pipeline with the Liliana adapter and a Static fetcher serving the
// fixture page and a fake cover.
func newTestAPI(t *testing.T, authToken string) http.Handler {
	t.Helper()
	h, _ := newTestAPIWithJobs(t, authToken, false)
	return h
}

// newTestAPIWithJobs optionally runs a real scheduler for POST /refresh.
func newTestAPIWithJobs(t *testing.T, authToken string, jobs bool) (http.Handler, *library.Service) {
	t.Helper()
	html, err := testfixture.ReadFile(t, "../../testdata/fixtures/manhuaplus.org/forging-an-industrial-empire-in-a-magical-world.html")
	if err != nil {
		t.Fatal(err)
	}
	f := &fetch.Static{Pages: map[string][]byte{
		seriesURL: html,
		coverURL:  []byte("\xff\xd8\xff\xe0jpeg"),
	}}
	clock := domain.FixedClock{T: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	store, err := storage.Open(context.Background(), t.TempDir(), clock, storage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ex := extract.New(f, extract.Options{Adapters: []extract.Adapter{liliana.New()}})
	svc := library.New(store, ex, clock, library.Options{Fetcher: f})
	settings := library.NewSettings(store, library.NotifyEnv{TelegramChatID: "42"})
	opts := Options{
		AuthToken: authToken, Version: "test",
		Recipes:  library.NewRecipes(store, recipe.NewBook(clock), f, clock),
		Settings: settings,
		Notifier: library.NewAnnouncer(svc, settings, nil, nil),
		Enricher: library.NewEnricher(svc, settings, nil, nil), // no finders: nothing is ever found
	}
	if jobs {
		sched := scheduler.New(svc, scheduler.Options{Interval: time.Hour})
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		go func() { _ = sched.Idle(ctx) }()
		time.Sleep(20 * time.Millisecond) // let Idle register as the job owner
		opts.Jobs = sched
	}
	return New(svc, store, opts), svc
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func (r response) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("decode %s: %v", r.body, err)
	}
	return m
}

func (r response) errorCode(t *testing.T) string {
	t.Helper()
	e, _ := r.json(t)["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func do(t *testing.T, h http.Handler, method, path, body string, hdr ...string) response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return response{status: rec.Code, header: rec.Header(), body: rec.Body.Bytes()}
}

func addSeries(t *testing.T, h http.Handler) string {
	t.Helper()
	r := do(t, h, "POST", "/api/v1/series", `{"url":"`+seriesURL+`"}`)
	if r.status != http.StatusCreated {
		t.Fatalf("add: %d %s", r.status, r.body)
	}
	return r.json(t)["id"].(string)
}

func TestHealthAndAuth(t *testing.T) {
	h := newTestAPI(t, token)
	if r := do(t, h, "GET", "/api/v1/health", ""); r.status != 200 || r.json(t)["version"] != "test" ||
		len(r.json(t)["solvers"].([]any)) != 0 {
		t.Errorf("health: %d %s", r.status, r.body)
	}
	if r := do(t, h, "GET", "/api/v1/series", ""); r.status != 401 || r.errorCode(t) != "unauthorized" {
		t.Errorf("no token: %d %s", r.status, r.body)
	}
	if r := do(t, h, "GET", "/api/v1/series", "", "Authorization", "Bearer wrong"); r.status != 401 {
		t.Errorf("wrong token: %d", r.status)
	}
	if r := do(t, h, "GET", "/api/v1/series", "", "Authorization", "Bearer "+token); r.status != 200 {
		t.Errorf("valid token: %d %s", r.status, r.body)
	}
	// Covers accept the token as a query parameter; other routes do not.
	if r := do(t, h, "GET", "/api/v1/covers/x?token="+token, ""); r.status != 404 {
		t.Errorf("cover with query token: %d, want 404 (authorized, no cover)", r.status)
	}
	if r := do(t, h, "GET", "/api/v1/covers/x?token=wrong", ""); r.status != 401 {
		t.Errorf("cover with wrong query token: %d", r.status)
	}
	if r := do(t, h, "GET", "/api/v1/series?token="+token, ""); r.status != 401 {
		t.Errorf("query token must only work for covers: %d", r.status)
	}
}

func TestGuardOrigin(t *testing.T) {
	h := newTestAPI(t, "")
	tests := []struct {
		name, method, origin string
		status               int
	}{
		{"same origin post", "POST", "http://example.com", 400}, // httptest Host is example.com
		{"cross origin post", "POST", "https://evil.test", 403},
		{"no origin (curl)", "POST", "", 400},
		{"cross origin get is allowed", "GET", "https://evil.test", 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/api/v1/series"
			hdr := []string{}
			if tt.origin != "" {
				hdr = append(hdr, "Origin", tt.origin)
			}
			r := do(t, h, tt.method, path, `{"url":"nope"}`, hdr...)
			if r.status != tt.status {
				t.Errorf("got %d %s, want %d", r.status, r.body, tt.status)
			}
		})
	}
}

func TestLoopbackOnly(t *testing.T) {
	h := New(nil, nil, Options{LoopbackOnly: true})
	for host, want := range map[string]int{
		"127.0.0.1:8730": 200, "localhost:8730": 200, "[::1]:8730": 200,
		"evil.test:8730": 403, "192.168.0.10:8730": 403,
	} {
		req := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/health", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("Host %s: got %d, want %d", host, rec.Code, want)
		}
	}
}

func TestAppMode_HealthAndQuit(t *testing.T) {
	quit := make(chan struct{})
	h := New(nil, nil, Options{Version: "v", OnQuit: func() { close(quit) }})
	if m := do(t, h, "GET", "/api/v1/health", "").json(t); m["mode"] != "app" {
		t.Errorf("health = %v", m)
	}
	if r := do(t, h, "POST", "/api/v1/app/quit", ""); r.status != 202 {
		t.Fatalf("quit: %d", r.status)
	}
	select {
	case <-quit:
	case <-time.After(time.Second):
		t.Fatal("OnQuit not called")
	}
	server := New(nil, nil, Options{})
	if m := do(t, server, "GET", "/api/v1/health", "").json(t); m["mode"] != "server" {
		t.Errorf("server health = %v", m)
	}
	if r := do(t, server, "POST", "/api/v1/app/show", ""); r.status != 404 {
		t.Errorf("show without OnShow = %d, want 404", r.status)
	}
	shown := make(chan struct{})
	desktop := New(nil, nil, Options{OnShow: func() { close(shown) }})
	if r := do(t, desktop, "POST", "/api/v1/app/show", ""); r.status != 202 {
		t.Errorf("show = %d", r.status)
	}
	<-shown
	if r := do(t, server, "POST", "/api/v1/app/quit", ""); r.status != 404 {
		t.Errorf("quit must not exist in server mode: %d", r.status)
	}
}

func TestPreviewDoesNotSave(t *testing.T) {
	h := newTestAPI(t, "")
	r := do(t, h, "POST", "/api/v1/extract/preview", `{"url":"`+seriesURL+`"}`)
	m := r.json(t)
	if r.status != 200 || m["method"] != "adapter:liliana" || m["needs_confirmation"] != false || len(m["chapters"].([]any)) != 81 {
		t.Errorf("preview: %d %v", r.status, m["method"])
	}
	if list := do(t, h, "GET", "/api/v1/series", "").json(t)["series"].([]any); len(list) != 0 {
		t.Error("preview saved a series")
	}
}

func TestSeriesLifecycle(t *testing.T) {
	h := newTestAPI(t, "")
	id := addSeries(t, h)

	got := do(t, h, "GET", "/api/v1/series/"+id, "").json(t)
	if got["unread_count"] != 81.0 || got["has_new"] != false || got["cover"].(map[string]any)["local"] != "covers/"+id+".jpg" {
		t.Errorf("get: unread=%v has_new=%v cover=%v", got["unread_count"], got["has_new"], got["cover"])
	}

	list := do(t, h, "GET", "/api/v1/series?q=industrial&sort=title", "").json(t)["series"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["chapters"] != nil {
		t.Errorf("list should have 1 series without chapters: %v", list)
	}
	if n := len(do(t, h, "GET", "/api/v1/series?source=manhuaplus.org", "").json(t)["series"].([]any)); n != 1 {
		t.Errorf("source filter: %d series", n)
	}
	if n := len(do(t, h, "GET", "/api/v1/series?source=other.org", "").json(t)["series"].([]any)); n != 0 {
		t.Errorf("other source: %d series", n)
	}
	sources := do(t, h, "GET", "/api/v1/sources", "").json(t)["sources"].([]any)
	if len(sources) != 1 || sources[0].(map[string]any)["domain"] != "manhuaplus.org" || sources[0].(map[string]any)["count"] != 1.0 {
		t.Errorf("sources = %v", sources)
	}

	if r := do(t, h, "PATCH", "/api/v1/series/"+id, `{"title":"Forging","category":"on_hold"}`); r.status != 200 || r.json(t)["title"] != "Forging" {
		t.Errorf("patch: %d %s", r.status, r.body)
	}

	r := do(t, h, "POST", "/api/v1/series/"+id+"/read", `{"up_to": 40}`)
	if r.status != 200 || r.json(t)["last_read_chapter"] != 40.0 || r.json(t)["unread_count"] != 40.0 {
		t.Errorf("read up_to: %d last=%v", r.status, r.json(t)["last_read_chapter"])
	}
	r = do(t, h, "POST", "/api/v1/series/"+id+"/read", `{"keys":["40"],"read":false}`)
	if r.json(t)["last_read_chapter"] != 39.0 {
		t.Errorf("unmark: last=%v", r.json(t)["last_read_chapter"])
	}

	r = do(t, h, "POST", "/api/v1/series/"+id+"/refresh", "")
	m := r.json(t)
	if r.status != 200 || m["new_chapters"] != 0.0 || m["series"].(map[string]any)["last_read_chapter"] != 39.0 {
		t.Errorf("refresh: %d %v", r.status, m["new_chapters"])
	}
	if r := do(t, h, "POST", "/api/v1/series/"+id+"/seen", ""); r.status != 200 {
		t.Errorf("seen: %d", r.status)
	}

	c := do(t, h, "GET", "/api/v1/covers/"+id, "")
	if c.status != 200 || c.header.Get("Content-Type") != "image/jpeg" || !bytes.HasPrefix(c.body, []byte("\xff\xd8")) {
		t.Errorf("cover: %d %q", c.status, c.header.Get("Content-Type"))
	}

	if r := do(t, h, "DELETE", "/api/v1/series/"+id, ""); r.status != 204 {
		t.Errorf("delete: %d", r.status)
	}
	if r := do(t, h, "GET", "/api/v1/series/"+id, ""); r.status != 404 || r.errorCode(t) != "not_found" {
		t.Errorf("after delete: %d %s", r.status, r.body)
	}
	if r := do(t, h, "GET", "/api/v1/covers/"+id, ""); r.status != 404 {
		t.Errorf("cover after delete: %d", r.status)
	}
}

func TestExportImport(t *testing.T) {
	src := newTestAPI(t, "")
	id := addSeries(t, src)
	do(t, src, "POST", "/api/v1/series/"+id+"/read", `{"up_to": 10}`)
	exp := do(t, src, "GET", "/api/v1/export", "")
	if exp.status != 200 || !strings.HasPrefix(exp.header.Get("Content-Disposition"), `attachment; filename="leftoff-`) {
		t.Fatalf("export: %d %v", exp.status, exp.header)
	}
	if m := exp.json(t); m["format"] != library.ExportFormat || len(m["series"].([]any)) != 1 {
		t.Errorf("export body = %v", m["format"])
	}

	dst := newTestAPI(t, "")
	r := do(t, dst, "POST", "/api/v1/import", string(exp.body))
	if r.status != 200 || r.json(t)["added"] != 1.0 {
		t.Fatalf("import: %d %s", r.status, r.body)
	}
	if got := do(t, dst, "GET", "/api/v1/series/"+id, "").json(t); got["last_read_chapter"] != 10.0 {
		t.Errorf("imported reading state = %v", got["last_read_chapter"])
	}
	for body, code := range map[string]string{
		`nope`:                         "invalid_input",
		`{"schema": 99, "series": []}`: "newer_schema",
	} {
		if r := do(t, dst, "POST", "/api/v1/import", body); r.errorCode(t) != code {
			t.Errorf("import %s: %d %s", body, r.status, r.body)
		}
	}
}

func TestCoverActions(t *testing.T) {
	h := newTestAPI(t, "")
	id := addSeries(t, h)
	c := do(t, h, "GET", "/api/v1/covers/"+id, "")
	etag := c.header.Get("ETag")
	if c.status != 200 || etag == "" || c.header.Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("cover: %d %v", c.status, c.header)
	}
	if r := do(t, h, "GET", "/api/v1/covers/"+id, "", "If-None-Match", etag); r.status != 304 {
		t.Errorf("revalidation: %d", r.status)
	}

	png := "\x89PNG\r\n\x1a\nmine"
	r := do(t, h, "PUT", "/api/v1/series/"+id+"/cover", png)
	cover, _ := r.json(t)["cover"].(map[string]any)
	if r.status != 200 || cover["custom"] != true || cover["local"] != "covers/"+id+".png" {
		t.Fatalf("upload: %d %s", r.status, r.body)
	}
	if r := do(t, h, "GET", "/api/v1/covers/"+id, "", "If-None-Match", etag); r.status != 200 || r.header.Get("Content-Type") != "image/png" {
		t.Errorf("new cover must not match the old ETag: %d %s", r.status, r.header.Get("Content-Type"))
	}
	if r := do(t, h, "PUT", "/api/v1/series/"+id+"/cover", "<html>"); r.status != 400 || r.errorCode(t) != "invalid_image" {
		t.Errorf("non-image: %d %s", r.status, r.body)
	}

	r = do(t, h, "POST", "/api/v1/series/"+id+"/cover/refresh", "")
	cover, _ = r.json(t)["cover"].(map[string]any)
	if r.status != 200 || cover["custom"] != false || cover["local"] != "covers/"+id+".jpg" {
		t.Errorf("refresh cover: %d %s", r.status, r.body)
	}
}

func TestChangeSource(t *testing.T) {
	h := newTestAPI(t, "")
	id := addSeries(t, h)
	// The fixture page is the only page the test fetcher serves: moving to
	// it again is not a move, and an unknown page is a 404.
	if r := do(t, h, "POST", "/api/v1/series/"+id+"/source", `{"url":"`+seriesURL+`"}`); r.status != 200 || len(r.json(t)["source"].(map[string]any)["mirrors"].([]any)) != 0 {
		t.Errorf("same address: %d %s", r.status, r.body)
	}
	if r := do(t, h, "POST", "/api/v1/series/"+id+"/source", `{"url":"https://manhuaplus.org/manga/gone"}`); r.status != 404 {
		t.Errorf("unknown page: %d %s", r.status, r.body)
	}
	if r := do(t, h, "POST", "/api/v1/series/"+id+"/source", `{"url":"nope"}`); r.errorCode(t) != "invalid_input" {
		t.Errorf("invalid url: %d %s", r.status, r.body)
	}
}

func TestRefreshAllJob(t *testing.T) {
	h, _ := newTestAPIWithJobs(t, "", true)
	addSeries(t, h)
	r := do(t, h, "POST", "/api/v1/refresh", "")
	if r.status != 202 || r.json(t)["status"] != "running" || r.json(t)["total"] != 1.0 {
		t.Fatalf("start: %d %s", r.status, r.body)
	}
	id := r.json(t)["id"].(string)
	deadline := time.Now().Add(5 * time.Second)
	for {
		j := do(t, h, "GET", "/api/v1/jobs/"+id, "").json(t)
		if j["status"] == "done" {
			if j["done"] != 1.0 || j["failed"] != 0.0 {
				t.Errorf("job = %v", j)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not finish: %v", j)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if r := do(t, h, "GET", "/api/v1/jobs/999", ""); r.status != 404 {
		t.Errorf("unknown job: %d", r.status)
	}
	noJobs := newTestAPI(t, "")
	if r := do(t, noJobs, "POST", "/api/v1/refresh", ""); r.status != 404 {
		t.Errorf("without a scheduler POST /refresh must not exist: %d", r.status)
	}
}

func TestRecipesAPI(t *testing.T) {
	h := newTestAPI(t, "")
	const rc = `{"schema":1,"domain":"manhuaplus.org","generated_by":"manual",
		"series":{"title":{"css":"article header h1"},"cover":{"css":"meta[property='og:image']","attr":"content"}},
		"chapters":{"item":{"css":"ul#myUL li.chapter"},"link":{"css":"a","attr":"href"},"label":{"css":"a"}}}`
	if r := do(t, h, "GET", "/api/v1/recipes", ""); r.status != 200 || len(r.json(t)["recipes"].([]any)) != 0 {
		t.Fatalf("empty list: %d %s", r.status, r.body)
	}
	test := `{"url":"` + seriesURL + `","recipe":` + rc + `}`
	r := do(t, h, "POST", "/api/v1/recipes/manhuaplus.org/test", test)
	res, _ := r.json(t)["result"].(map[string]any)
	if r.status != 200 || len(r.json(t)["problems"].([]any)) != 0 || len(res["chapters"].([]any)) != 81 {
		t.Fatalf("test: %d %s", r.status, r.body)
	}
	if r := do(t, h, "PUT", "/api/v1/recipes/manhuaplus.org", `{"schema":1}`); r.status != 400 || r.errorCode(t) != "invalid_input" {
		t.Errorf("invalid recipe: %d %s", r.status, r.body)
	}
	if r := do(t, h, "PUT", "/api/v1/recipes/manhuaplus.org", rc); r.status != 204 {
		t.Fatalf("put: %d %s", r.status, r.body)
	}
	if r := do(t, h, "GET", "/api/v1/recipes/manhuaplus.org", ""); r.status != 200 || !strings.Contains(string(r.body), "ul#myUL") {
		t.Errorf("get: %d %s", r.status, r.body)
	}
	if list := do(t, h, "GET", "/api/v1/recipes", "").json(t)["recipes"].([]any); len(list) != 1 {
		t.Errorf("list = %v", list)
	}
	if r := do(t, h, "DELETE", "/api/v1/recipes/manhuaplus.org", ""); r.status != 204 {
		t.Errorf("delete: %d", r.status)
	}
	if r := do(t, h, "GET", "/api/v1/recipes/manhuaplus.org", ""); r.status != 404 {
		t.Errorf("get after delete: %d", r.status)
	}
	if r := do(t, h, "POST", "/api/v1/recipes/x.org/test", `{"url":"`+seriesURL+`"}`); r.errorCode(t) != "invalid_input" {
		t.Errorf("test without recipe: %d %s", r.status, r.body)
	}
}

func TestRequestErrors(t *testing.T) {
	h := newTestAPI(t, "")
	id := addSeries(t, h)
	tests := []struct {
		name, method, path, body string
		status                   int
		code                     string
	}{
		{"duplicate", "POST", "/api/v1/series", `{"url":"` + seriesURL + `/"}`, 409, "duplicate"},
		{"invalid url", "POST", "/api/v1/series", `{"url":"nope"}`, 400, "invalid_input"},
		{"page not found", "POST", "/api/v1/extract/preview", `{"url":"https://manhuaplus.org/manga/missing"}`, 404, "not_found"},
		{"malformed json", "POST", "/api/v1/series", `{"url":`, 400, "invalid_input"},
		{"unknown field", "PATCH", "/api/v1/series/" + id, `{"titel":"x"}`, 400, "invalid_input"},
		{"bad type", "PATCH", "/api/v1/series/" + id, `{"type":"comic"}`, 400, "invalid_input"},
		{"read without target", "POST", "/api/v1/series/" + id + "/read", `{}`, 400, "invalid_input"},
		{"read with both", "POST", "/api/v1/series/" + id + "/read", `{"up_to":1,"keys":["1"]}`, 400, "invalid_input"},
		{"unknown chapter", "POST", "/api/v1/series/" + id + "/read", `{"keys":["999"]}`, 404, "not_found"},
		{"bad has_new", "GET", "/api/v1/series?has_new=maybe", "", 400, "invalid_input"},
		{"bad sort", "GET", "/api/v1/series?sort=random", "", 400, "invalid_input"},
		{"unknown series", "POST", "/api/v1/series/nope/refresh", "", 404, "not_found"},
		{"unknown endpoint", "GET", "/api/v1/nothing", "", 404, "not_found"},
		{"body too large", "POST", "/api/v1/series", `{"url":"` + strings.Repeat("a", maxBodyBytes) + `"}`, 413, "too_large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := do(t, h, tt.method, tt.path, tt.body)
			if r.status != tt.status || r.errorCode(t) != tt.code {
				t.Errorf("got %d %s, want %d %s", r.status, r.body, tt.status, tt.code)
			}
		})
	}
}

func TestWriteError_Mapping(t *testing.T) {
	s := &server{log: slog.New(slog.DiscardHandler)}
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{fmt.Errorf("x: %w", domain.ErrBlocked), 502, "blocked"},
		{errors.Join(fmt.Errorf("x: %w", domain.ErrBlocked), fmt.Errorf("browser: %w", fetch.ErrNoBrowser)), 502, "no_browser"},
		{fmt.Errorf("x: %w", domain.ErrUnsupportedSite), 422, "unsupported_site"},
		{fmt.Errorf("x: %w", extract.ErrIncomplete), 422, "extraction_incomplete"},
		{fmt.Errorf("x: %w", library.ErrSourceShrank), 502, "source_changed"},
		{&fetch.StatusError{URL: "u", StatusCode: 500}, 502, "upstream_error"},
		{fmt.Errorf("x: %w", context.DeadlineExceeded), 504, "timeout"},
		{errors.New("disk on fire"), 500, "internal"},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		s.writeError(rec, httptest.NewRequestWithContext(context.Background(), "GET", "/", nil), tt.err)
		r := response{status: rec.Code, body: rec.Body.Bytes()}
		if r.status != tt.status || r.errorCode(t) != tt.code {
			t.Errorf("%v: got %d %s", tt.err, r.status, r.body)
		}
		if tt.code == "internal" && strings.Contains(string(r.body), "disk on fire") {
			t.Error("internal error details leaked to the client")
		}
	}
}
