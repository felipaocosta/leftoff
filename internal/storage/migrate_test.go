package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
)

func doc(t *testing.T, s string) map[string]json.RawMessage {
	t.Helper()
	var d map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestMigrate(t *testing.T) {
	renameTitle := func(d map[string]json.RawMessage) error {
		d["name"] = d["title"]
		delete(d, "title")
		return nil
	}
	failing := func(map[string]json.RawMessage) error { return errors.New("boom") }

	tests := []struct {
		name         string
		in           string
		target       int
		steps        []migration
		wantMigrated bool
		wantErr      error
		wantAnyErr   bool
		wantKey      string
	}{
		{name: "already current", in: `{"schema":1}`, target: 1},
		{name: "one step", in: `{"schema":1,"title":"x"}`, target: 2, steps: []migration{renameTitle}, wantMigrated: true, wantKey: "name"},
		{name: "newer schema refused", in: `{"schema":3}`, target: 2, wantErr: ErrNewerSchema},
		{name: "missing schema", in: `{}`, target: 1, wantAnyErr: true},
		{name: "invalid schema", in: `{"schema":"one"}`, target: 1, wantAnyErr: true},
		{name: "missing step", in: `{"schema":1}`, target: 3, steps: []migration{renameTitle}, wantAnyErr: true},
		{name: "step fails", in: `{"schema":1}`, target: 2, steps: []migration{failing}, wantAnyErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := doc(t, tt.in)
			migrated, err := migrate(d, tt.target, tt.steps)
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			case tt.wantAnyErr:
				if err == nil {
					t.Fatal("expected error")
				}
				return
			case err != nil:
				t.Fatal(err)
			}
			if migrated != tt.wantMigrated {
				t.Errorf("migrated = %v, want %v", migrated, tt.wantMigrated)
			}
			if got, _ := schemaOf(d); got != tt.target {
				t.Errorf("schema = %d, want %d", got, tt.target)
			}
			if _, ok := d[tt.wantKey]; tt.wantKey != "" && !ok {
				t.Errorf("key %q missing after migration", tt.wantKey)
			}
		})
	}
}

// TestOpen_MigratesTheRealV1FileToV2 runs the real migration on a real v1
// library (RULES §7): schema 2, a backup of the v1 file, every series,
// read state and unknown field kept, and external unset.
func TestOpen_MigratesTheRealV1FileToV2(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, "library-v1.json", dir)
	original, err := os.ReadFile(filepath.Join(dir, libraryFile))
	if err != nil {
		t.Fatal(err)
	}
	s := openStore(t, dir, domain.FixedClock{T: t0})
	lib := s.Library()
	if lib.Schema != CurrentSchema || len(lib.Series) != 1 || lib.Series[0].External != nil || !lib.Series[0].Chapters[1].Read {
		t.Fatalf("migrated library = schema %d, %d series, external %v", lib.Schema, len(lib.Series), lib.Series[0].External)
	}
	backups, err := listBackups(filepath.Join(dir, backupsDir))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v, %v", backups, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, backupsDir, backups[0])); string(b) != string(original) {
		t.Error("the backup is not the v1 file")
	}
	if err := s.Update(context.Background(), func(*Library) error { return nil }); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, libraryFile))
	for _, want := range []string{`"schema": 3`, `"external": null`, `"sync_token": "abc"`, `"rating_from_future_version": 5`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("saved file lacks %s", want)
		}
	}
}

func TestMigrateV1ToV2_RejectsABrokenSeriesList(t *testing.T) {
	if err := migrateV1ToV2(doc(t, `{"schema":1,"series":{}}`)); err == nil {
		t.Error("series must be a list")
	}
}

// TestOpen_MigratesTheRealV2FileToV3 uses a library written by the schema 2
// code (testdata/storage/library-v2.json): the stored check error gets the
// code its message implies, and nothing else changes.
func TestOpen_MigratesTheRealV2FileToV3(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, "library-v2.json", dir)
	original, err := os.ReadFile(filepath.Join(dir, libraryFile))
	if err != nil {
		t.Fatal(err)
	}
	lib := openStore(t, dir, domain.FixedClock{T: t0}).Library()
	if lib.Schema != 3 || len(lib.Series) != 2 {
		t.Fatalf("schema %d, %d series", lib.Schema, len(lib.Series))
	}
	gendai, eleceed := lib.Series[0], lib.Series[1]
	if ce := gendai.CheckError; ce == nil || ce.Code != domain.CheckSourceChanged || ce.Failures != 4 || !strings.Contains(ce.Message, "0 of 2") {
		t.Errorf("check_error = %+v", ce)
	}
	if !gendai.Chapters[0].Read || gendai.External == nil || gendai.External.ID != "1" || string(gendai.Extra["from_a_future_version"]) != "true" {
		t.Errorf("series changed: %+v", gendai)
	}
	if eleceed.CheckError != nil {
		t.Errorf("a series without errors got one: %+v", eleceed.CheckError)
	}
	backups, _ := listBackups(filepath.Join(dir, backupsDir))
	if len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, backupsDir, backups[0])); string(b) != string(original) {
		t.Error("the backup is not the v2 file")
	}
}

func TestMigrateV2ToV3_CodesLegacyMessages(t *testing.T) {
	for msg, want := range map[string]string{
		"refresh x: source returned suspiciously few chapters: 0 of 50": "source_changed",
		"fetch https://a.org: site unreachable: dial tcp: refused":      "upstream_error",
		"heuristic: series title: required field not found":             "extraction_incomplete",
		"something else": "",
	} {
		doc := map[string]json.RawMessage{"schema": json.RawMessage(`2`),
			"series": json.RawMessage(`[{"id":"a","check_error":{"message":` + strconv.Quote(msg) + `,"failures":1}},{"id":"b","check_error":null}]`)}
		if err := migrateV2ToV3(doc); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(doc["series"]), `"code":"`+want+`"`) || !strings.Contains(string(doc["series"]), `"check_error":null`) {
			t.Errorf("%q → %s", msg, doc["series"])
		}
	}
}
