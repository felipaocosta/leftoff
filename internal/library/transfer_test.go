package library

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/storage"
)

func exportJSON(t *testing.T, e *env) []byte {
	t.Helper()
	b, err := json.Marshal(e.svc.Export())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestExportImport_RoundTripIntoEmptyLibrary(t *testing.T) {
	src := newEnv(t)
	s := src.add(t)
	if _, err := src.svc.MarkReadUpTo(context.Background(), s.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := src.svc.Edit(context.Background(), s.ID, Patch{Notes: ptr("great"), Tags: ptr([]string{"fav"})}); err != nil {
		t.Fatal(err)
	}
	data := exportJSON(t, src)

	dst := newEnv(t)
	report, err := dst.svc.Import(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	if report != (ImportReport{Added: 1}) {
		t.Errorf("report = %+v", report)
	}
	got, err := dst.svc.Get(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Notes != "great" || got.Tags[0] != "fav" || *got.LastReadChapter != 1 || got.UnreadCount() != 1 || got.Cover.Local != "" {
		t.Errorf("imported = %+v", got)
	}
}

func TestImport_MergesReadingStateIntoExistingSeries(t *testing.T) {
	dst := newEnv(t)
	ctx := context.Background()
	mine := dst.add(t)
	if _, err := dst.svc.Edit(ctx, mine.ID, Patch{Title: ptr("My title"), Notes: ptr("keep")}); err != nil {
		t.Fatal(err)
	}
	early := t0.Add(-48 * time.Hour)
	other := mine // same work exported from another device, different id
	other.ID = "01OTHERDEVICE0000000000000"
	other.Title, other.Notes = "Their title", "theirs"
	other.Source.URL = "https://www.site.org/manga/x/" // same canonical URL
	other.Chapters = []domain.Chapter{
		{Key: "2", Number: num(2), Label: "Chapter 2", Read: true, ReadAt: &early},
		{Key: "0.5", Number: num(0.5), Label: "Prologue", Read: true, ReadAt: &early}, // only in the export
		{Key: "1", Number: num(1), Label: "Chapter 1", Read: false},
	}
	data, _ := json.Marshal(ExportFile{Format: ExportFormat, Schema: 1, Series: []domain.Series{other}})

	report, err := dst.svc.Import(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if report != (ImportReport{Merged: 1, ChaptersRead: 2}) {
		t.Errorf("report = %+v", report)
	}
	got, _ := dst.svc.Get(mine.ID)
	if got.Title != "My title" || got.Notes != "keep" || len(dst.svc.List()) != 1 {
		t.Errorf("existing fields must win: %+v", got)
	}
	byKey := map[string]domain.Chapter{}
	for _, c := range got.Chapters {
		byKey[c.Key] = c
	}
	if !byKey["2"].Read || !byKey["2"].ReadAt.Equal(early) || !byKey["0.5"].Read || byKey["1"].Read {
		t.Errorf("chapters = %+v", got.Chapters)
	}
}

func TestImport_IDs(t *testing.T) {
	dst := newEnv(t)
	mine := dst.add(t)

	// Same id: the same series exported earlier, even if its URL changed.
	moved := mine
	moved.Source.URL = "https://new-domain.org/manga/x"
	data, _ := json.Marshal(ExportFile{Format: ExportFormat, Schema: 1, Series: []domain.Series{moved}})
	if report, err := dst.svc.Import(context.Background(), data); err != nil || report.Merged != 1 {
		t.Fatalf("same id should merge: %+v %v", report, err)
	}

	// No id (e.g. a hand-written file): a new one is generated.
	other := mine
	other.ID, other.Title, other.Source.URL = "", "Z", "https://another.org/manga/z"
	data, _ = json.Marshal(ExportFile{Format: ExportFormat, Schema: 1, Series: []domain.Series{other}})
	if report, err := dst.svc.Import(context.Background(), data); err != nil || report.Added != 1 {
		t.Fatalf("missing id should add: %+v %v", report, err)
	}
	if list := dst.svc.List(); len(list) != 2 || len(list[1].ID) != 26 || list[1].ID == mine.ID {
		t.Errorf("ids = %s, %s", list[0].ID, list[1].ID)
	}
}

func TestImport_AcceptsRawLibraryJSONAndTakesABackup(t *testing.T) {
	dst := newEnv(t)
	dst.add(t)
	raw, err := testfixture.ReadFile(t, "../../testdata/storage/library-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	report, err := dst.svc.Import(context.Background(), raw)
	if err != nil || report.Added != 1 {
		t.Fatalf("report = %+v, err = %v", report, err)
	}
	backups, _ := os.ReadDir(filepath.Join(dst.dir, "backups"))
	if len(backups) == 0 {
		t.Error("import must back up the library first")
	}
}

func TestImport_RejectsInvalidFiles(t *testing.T) {
	dst := newEnv(t)
	tests := []struct {
		name    string
		data    string
		wantErr error
	}{
		{"not json", `hello`, domain.ErrInvalidInput},
		{"other format", `{"format":"mihon","schema":1,"series":[]}`, domain.ErrInvalidInput},
		{"no series", `{"schema":1}`, domain.ErrInvalidInput},
		{"newer schema", `{"schema":99,"series":[]}`, storage.ErrNewerSchema},
		{"series without title", `{"schema":1,"series":[{"id":"x","source":{"url":"https://a.org/x"}}]}`, domain.ErrInvalidInput},
		{"source not http", `{"schema":1,"series":[{"title":"T","source":{"url":"0"}}]}`, domain.ErrInvalidInput},
		{"source javascript", `{"schema":1,"series":[{"title":"T","source":{"url":"javascript:alert(1)"}}]}`, domain.ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := dst.svc.Import(context.Background(), []byte(tt.data)); !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
	if len(dst.svc.List()) != 0 {
		t.Error("rejected imports must not change the library")
	}
}

func TestImport_NewSeriesGetsOneChapterPerKey(t *testing.T) {
	dst := newEnv(t)
	data := `{"schema":2,"series":[{"title":"T","source":{"url":"https://a.org/t"},"chapters":[
		{"key":"2","number":2,"label":"Chapter 2","read":false},
		{"key":"2","number":2,"label":"Chapter 2","read":true,"read_at":"2026-10-01T00:00:00Z"},
		{"number":1,"label":"Chapter 1","read":true}]}]}`
	if _, err := dst.svc.Import(context.Background(), []byte(data)); err != nil {
		t.Fatal(err)
	}
	got := dst.svc.List()[0]
	m := byKey(t, got.Chapters)
	if len(m) != 2 || !m["2"].Read || m["2"].ReadAt == nil || !m["1"].Read || got.Source.Domain != "a.org" {
		t.Errorf("chapters = %+v, domain = %q", got.Chapters, got.Source.Domain)
	}
}

func TestImport_AcceptsExportsFromBeforeTheRename(t *testing.T) {
	dst := newEnv(t)
	data := `{"format":"manga-tracker-export","schema":2,"series":[{"title":"T","source":{"url":"https://a.org/t"}}]}`
	if r, err := dst.svc.Import(context.Background(), []byte(data)); err != nil || r.Added != 1 {
		t.Errorf("import = %+v, %v", r, err)
	}
}
