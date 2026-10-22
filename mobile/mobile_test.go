package mobile

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// call does one request and returns the status and body.
func call(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

func TestStartTargetStop(t *testing.T) {
	dir := t.TempDir()
	url, err := Start(dir)
	if err != nil || !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("start: %q, %v", url, err)
	}
	defer Stop()
	if again, _ := Start(dir); again != url || LocalURL() != url {
		t.Errorf("a second Start must reuse the server: %q", again)
	}
	if _, body := call(t, http.MethodGet, url+"api/v1/health", ""); !strings.Contains(body, `"mode":"android"`) {
		t.Errorf("health = %s", body)
	}
	if TargetURL() != url {
		t.Errorf("standalone target = %q", TargetURL())
	}
	if code, body := call(t, http.MethodPut, url+"api/v1/settings", `{"desktop":{"mode":"connected","server_url":"http://nas:8080"}}`); code != 200 {
		t.Fatalf("put settings: %d %s", code, body)
	}
	if got := TargetURL(); got != "http://nas:8080/" {
		t.Errorf("connected target = %q", got)
	}
	if code, _ := call(t, http.MethodPost, url+"api/v1/app/quit", ""); code != http.StatusNotFound {
		t.Errorf("Android has no quit route: %d", code)
	}
	Stop()
	if LocalURL() != "" || TargetURL() != "" {
		t.Error("Stop must forget the server")
	}
}

func TestCheckNow_EmptyLibrary(t *testing.T) {
	got, err := CheckNow(t.TempDir())
	if err != nil || got != "[]" {
		t.Errorf("CheckNow = %q, %v", got, err)
	}
	if LocalURL() != "" {
		t.Error("CheckNow must stop the server it started")
	}
}

func TestNewSince(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"series":[
			{"id":"a","title":"Old","latest_chapter":3,"last_new_chapter_at":"2026-10-20T10:00:00Z"},
			{"id":"b","title":"Fresh","latest_chapter":9,"last_new_chapter_at":"2026-10-21T12:00:01Z"},
			{"id":"c","title":"Never","latest_chapter":null,"last_new_chapter_at":null}]}`))
	}))
	defer srv.Close()
	got, err := newSince(context.Background(), srv.URL+"/", time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	if err != nil || len(got) != 1 || got[0].Title != "Fresh" || *got[0].LatestChapter != 9 {
		t.Errorf("newSince = %+v, %v", got, err)
	}
}

func TestCheckNow_KeepsTheAppServerRunning(t *testing.T) {
	dir := t.TempDir()
	base, err := Start(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer Stop()
	if _, err := CheckNow(dir); err != nil {
		t.Fatal(err)
	}
	if LocalURL() != base {
		t.Errorf("LocalURL = %q, want the app's server %q", LocalURL(), base)
	}
}

// fakeBrowser stands for the WebView: every page is plain and clear.
type fakeBrowser struct{ loads []string }

func (f *fakeBrowser) Load(u string, _ int64) error { f.loads = append(f.loads, u); return nil }
func (f *fakeBrowser) Eval(js string) (string, error) {
	if js == "document.documentElement.outerHTML" {
		return `"<html><h1>Rendered</h1></html>"`, nil
	}
	return "null", nil
}
func (f *fakeBrowser) Show(string) bool      { return false }
func (f *fakeBrowser) Hide()                 {}
func (f *fakeBrowser) Cookies(string) string { return "" }
func (f *fakeBrowser) UserAgent() string     { return "WebView" }

func TestSetBrowser_TheCoreReportsTheWebView(t *testing.T) {
	SetBrowser(&fakeBrowser{})
	defer SetBrowser(nil)
	base, err := Start(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer Stop()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }() // read below
	var h struct {
		Solvers []string `json:"solvers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil || !slices.Contains(h.Solvers, "webview") {
		t.Errorf("solvers = %v, %v", h.Solvers, err)
	}
}
