package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
	"github.com/felipaocosta/leftoff/internal/library"
	"github.com/felipaocosta/leftoff/internal/storage"
)

func TestSaveExport_DesktopDialog(t *testing.T) {
	clock := domain.FixedClock{T: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)}
	store, err := storage.Open(context.Background(), t.TempDir(), clock, storage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	f := &fetch.Static{}
	svc := library.New(store, extract.New(f, extract.Options{}), clock, library.Options{Fetcher: f})
	var gotName string
	var gotData []byte
	cancel := false
	save := func(_ context.Context, name string, data []byte) (string, error) {
		if cancel {
			return "", ErrSaveCanceled
		}
		gotName, gotData = name, data
		return "/home/u/" + name, nil
	}
	h := New(svc, store, Options{SaveFile: save})

	r := do(t, h, "POST", "/api/v1/app/export", "")
	if r.status != 200 || r.json(t)["path"] != "/home/u/leftoff-20261021-120000.json" || gotName != "leftoff-20261021-120000.json" {
		t.Fatalf("save: %d %s", r.status, r.body)
	}
	var file map[string]any
	if err := json.Unmarshal(gotData, &file); err != nil || file["format"] != "leftoff-export" {
		t.Errorf("saved data = %.80s (%v)", gotData, err)
	}
	cancel = true
	if r := do(t, h, "POST", "/api/v1/app/export", ""); r.status != 200 || r.json(t)["canceled"] != true {
		t.Errorf("canceled: %d %s", r.status, r.body)
	}
	if r := do(t, New(svc, store, Options{}), "POST", "/api/v1/app/export", ""); r.status != 404 {
		t.Errorf("without SaveFile the route must not exist: %d", r.status)
	}
	h = New(svc, store, Options{SaveFile: func(context.Context, string, []byte) (string, error) {
		return "", errors.New("disk full")
	}})
	if r := do(t, h, "POST", "/api/v1/app/export", ""); r.status != 500 || strings.Contains(string(r.body), "disk full") {
		t.Errorf("failure: %d %s", r.status, r.body)
	}
}
