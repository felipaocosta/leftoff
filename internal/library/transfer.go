package library

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/storage"
)

// ExportFormat identifies a LeftOff export file (RF-43).
const ExportFormat = "leftoff-export"

// legacyExportFormat is the format of exports made before the app was
// renamed to LeftOff; they still import.
const legacyExportFormat = "manga-tracker-export"

// ExportFile is the portable backup of the whole library. Covers are not
// included: they are downloaded again on the next refresh.
type ExportFile struct {
	Format     string          `json:"format"`
	Schema     int             `json:"schema"`
	ExportedAt time.Time       `json:"exported_at"`
	Series     []domain.Series `json:"series"`
}

// ImportReport summarizes an import.
type ImportReport struct {
	Added        int `json:"added"`         // series that were not in the library
	Merged       int `json:"merged"`        // series already present, reading state combined
	ChaptersRead int `json:"chapters_read"` // chapters newly marked read by the import
}

// Export returns the library as an ExportFile.
func (s *Service) Export() ExportFile {
	lib := s.store.Library()
	return ExportFile{Format: ExportFormat, Schema: lib.Schema, ExportedAt: s.clock.Now(), Series: lib.Series}
}

// importFile accepts an ExportFile or a raw library.json (from backups/).
type importFile struct {
	Format string          `json:"format"`
	Schema int             `json:"schema"`
	Series []domain.Series `json:"series"`
}

// Import merges an export (or library.json) into the library after taking
// a backup. Series already present (same id, or same canonical source URL or
// mirror) keep their fields and gain the imported reading state: a chapter
// read on either side ends up read, with the earliest read_at. New series
// are added as they are, without the local cover file.
func (s *Service) Import(ctx context.Context, data []byte) (ImportReport, error) {
	file, err := parseImport(data)
	if err != nil {
		return ImportReport{}, err
	}
	if err := s.store.Backup(ctx); err != nil {
		return ImportReport{}, fmt.Errorf("backup before import: %w", err)
	}
	var report ImportReport
	err = s.store.Update(ctx, func(lib *storage.Library) error {
		report = ImportReport{}
		for _, in := range file.Series {
			if i := findSame(lib, in); i >= 0 {
				report.ChaptersRead += mergeReading(&lib.Series[i], in)
				report.Merged++
				continue
			}
			added, err := s.prepareImported(lib, in)
			if err != nil {
				return err
			}
			lib.Series = append(lib.Series, added)
			report.Added++
		}
		return nil
	})
	if err != nil {
		return ImportReport{}, err
	}
	return report, nil
}

func parseImport(data []byte) (*importFile, error) {
	var file importFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("import: not a LeftOff export: %w: %w", err, domain.ErrInvalidInput)
	}
	if file.Format != "" && file.Format != ExportFormat && file.Format != legacyExportFormat {
		return nil, fmt.Errorf("import: unknown format %q: %w", file.Format, domain.ErrInvalidInput)
	}
	if file.Schema < 1 || file.Series == nil {
		return nil, fmt.Errorf("import: missing schema or series: %w", domain.ErrInvalidInput)
	}
	if file.Schema > storage.CurrentSchema {
		return nil, fmt.Errorf("import: schema %d is newer than this app: %w", file.Schema, storage.ErrNewerSchema)
	}
	for i, sr := range file.Series {
		if strings.TrimSpace(sr.Title) == "" || sr.Source.URL == "" {
			return nil, fmt.Errorf("import: series #%d has no title or source url: %w", i+1, domain.ErrInvalidInput)
		}
		if u, err := url.Parse(sr.Source.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("import: series #%d source url is not http(s): %w", i+1, domain.ErrInvalidInput)
		}
		file.Series[i].Chapters = uniqueChapters(sr.Chapters)
	}
	return &file, nil
}

// findSame returns the index of the library series that is the same work as
// in (same id or same canonical URL), or -1.
func findSame(lib *storage.Library, in domain.Series) int {
	if i := lib.SeriesByID(in.ID); i >= 0 {
		return i
	}
	keys := map[string]bool{}
	for _, raw := range append([]string{in.Source.URL}, in.Source.Mirrors...) {
		if u, err := url.Parse(raw); err == nil {
			keys[extract.CanonicalKey(u)] = true
		}
	}
	for i, sr := range lib.Series {
		for _, raw := range append([]string{sr.Source.URL}, sr.Source.Mirrors...) {
			if u, err := url.Parse(raw); err == nil && keys[extract.CanonicalKey(u)] {
				return i
			}
		}
	}
	return -1
}

// mergeReading copies the reading state of in into sr: chapters read in in
// become read in sr (earliest read_at kept), and chapters only known to in
// are added if they were read, so no reading history is lost. It returns
// how many chapters became read.
func mergeReading(sr *domain.Series, in domain.Series) int {
	index := make(map[string]int, len(sr.Chapters))
	for i, c := range sr.Chapters {
		index[c.Key] = i
	}
	marked := 0
	for _, c := range in.Chapters {
		if !c.Read {
			continue
		}
		i, ok := index[c.Key]
		if !ok {
			sr.Chapters = append(sr.Chapters, c)
			index[c.Key] = len(sr.Chapters) - 1
			marked++
			continue
		}
		if !sr.Chapters[i].Read {
			marked++
		}
		sr.Chapters[i] = mergeReadState(sr.Chapters[i], c)
	}
	if sr.Notes == "" {
		sr.Notes = in.Notes
	}
	if len(sr.Tags) == 0 && len(in.Tags) > 0 {
		sr.Tags = in.Tags
	}
	return marked
}

// prepareImported makes an imported series safe to add: a fresh id if the
// imported one is taken or invalid, no local cover file, non-nil slices.
func (s *Service) prepareImported(lib *storage.Library, in domain.Series) (domain.Series, error) {
	if in.ID == "" || lib.SeriesByID(in.ID) >= 0 {
		id, err := s.newID()
		if err != nil {
			return domain.Series{}, err
		}
		in.ID = id
	}
	if !in.Type.Valid() {
		in.Type = domain.TypeOther
	}
	if !in.Category.Valid() {
		in.Category = domain.CategoryReading
	}
	if in.AddedAt.IsZero() {
		in.AddedAt = s.clock.Now()
	}
	in.Cover.Local = "" // the file is not part of the export; refresh downloads it
	in.AltTitles, in.Tags, in.Source.Mirrors = nonNil(in.AltTitles), nonNil(in.Tags), nonNil(in.Source.Mirrors)
	if u, err := url.Parse(in.Source.URL); err == nil && in.Source.Domain == "" {
		in.Source.Domain = extract.Domain(u)
	}
	return in, nil
}

// uniqueChapters gives every imported chapter a key and keeps one chapter
// per key (the first, read if any copy was read), as the rest of the
// library expects.
func uniqueChapters(chs []domain.Chapter) []domain.Chapter {
	out := make([]domain.Chapter, 0, len(chs))
	index := make(map[string]int, len(chs))
	for _, c := range chs {
		if c.Key == "" {
			c.Key = domain.ChapterKey(c.Number, c.Label)
		}
		c.AltURLs = nonNil(c.AltURLs)
		if i, dup := index[c.Key]; dup {
			if c.Read {
				out[i] = mergeReadState(out[i], c)
			}
			continue
		}
		index[c.Key] = len(out)
		out = append(out, c)
	}
	return out
}
