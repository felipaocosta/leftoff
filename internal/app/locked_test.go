package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLockedPage_TakeoverFreesTheFolderAndOpens(t *testing.T) {
	dir := t.TempDir()
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- Serve(context.Background(), Config{DataDir: dir, MinInterval: time.Millisecond}, discard(),
			ServeOptions{Host: "127.0.0.1", AppMode: true, Version: "old", OnReady: func(a string) { ready <- a }})
	}()
	<-ready

	opened := make(chan struct{})
	page := httptest.NewServer(LockedPage("pt-BR", "PID 1, app old", dir, func() { close(opened) }))
	defer page.Close()
	body := get(t, page.URL+"/")
	if !strings.Contains(body, "Encerrar o outro e abrir este") || !strings.Contains(body, "PID 1, app old") {
		t.Errorf("page = %s", body)
	}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, page.URL+"/takeover", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close() // status only
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("takeover = %d", resp.StatusCode)
	}
	select {
	case <-opened:
	case <-time.After(5 * time.Second):
		t.Fatal("this app was not opened")
	}
	if err := <-done; err != nil {
		t.Errorf("the other app: %v", err)
	}
}

func TestSystemLanguage(t *testing.T) {
	for env, want := range map[string]string{"pt_BR.UTF-8": "pt-BR", "en_US.UTF-8": "en", "": "en"} {
		got := SystemLanguage(func(k string) string {
			if k == "LANG" {
				return env
			}
			return ""
		})
		if got != want {
			t.Errorf("LANG=%q → %s, want %s", env, got, want)
		}
	}
}

func get(t *testing.T, url string) string {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }() // read below
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
