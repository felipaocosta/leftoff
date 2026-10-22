package library

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
)

var (
	t0 = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	t1 = t0.Add(6 * time.Hour)
)

func num(v float64) *float64 { return &v }

// stored builds a stored chapter; read chapters get read_at = t0.
func stored(n float64, url string, read bool) domain.Chapter {
	c := fromInfo(domain.ChapterInfo{Number: num(n), Label: "Chapter", URL: url})
	if read {
		at := t0
		c.Read, c.ReadAt = true, &at
	}
	return c
}

func info(n float64, url string) domain.ChapterInfo {
	return domain.ChapterInfo{Number: num(n), Label: "Chapter", URL: url}
}

func byKey(t *testing.T, chs []domain.Chapter) map[string]domain.Chapter {
	t.Helper()
	m := map[string]domain.Chapter{}
	for _, c := range chs {
		if _, dup := m[c.Key]; dup {
			t.Fatalf("duplicate key %q in %+v", c.Key, chs)
		}
		m[c.Key] = c
	}
	return m
}

func TestMergeChapters_NewChapterIsUnreadAndDiscovered(t *testing.T) {
	old := []domain.Chapter{stored(1, "https://a.org/c/1", true)}
	res, err := mergeChapters(old, []domain.ChapterInfo{info(2, "https://a.org/c/2"), info(1, "https://a.org/c/1")}, t1)
	if err != nil {
		t.Fatal(err)
	}
	m := byKey(t, res.Chapters)
	if res.New != 1 || m["2"].Read || m["2"].DiscoveredAt == nil || !m["2"].DiscoveredAt.Equal(t1) {
		t.Errorf("new chapter = %+v (new=%d)", m["2"], res.New)
	}
	if !m["1"].Read || !m["1"].ReadAt.Equal(t0) || m["1"].DiscoveredAt != nil {
		t.Errorf("existing chapter lost state: %+v", m["1"])
	}
	if res.Chapters[0].Key != "2" {
		t.Error("output must follow source order")
	}
}

func TestMergeChapters_RemovedFromSource(t *testing.T) {
	old := []domain.Chapter{
		stored(3, "https://a.org/c/3", false),
		stored(2, "https://a.org/c/2", true),  // read: must survive
		stored(1, "https://a.org/c/1", false), // unread: nothing to lose
	}
	res, err := mergeChapters(old, []domain.ChapterInfo{info(3, "https://a.org/c/3")}, t1)
	if err != nil {
		t.Fatal(err)
	}
	m := byKey(t, res.Chapters)
	if _, ok := m["2"]; !ok || !m["2"].Read {
		t.Error("read chapter removed from source was dropped")
	}
	if _, ok := m["1"]; ok {
		t.Error("unread chapter removed from source should be dropped")
	}
	if res.Chapters[len(res.Chapters)-1].Key != "2" {
		t.Error("kept removed chapters go after the source order")
	}
}

func TestMergeChapters_RenumberingFollowsURL(t *testing.T) {
	// The source inserted a chapter 1 and shifted everything: the content
	// previously numbered 1 and 2 is now 2 and 3, at the same URLs.
	old := []domain.Chapter{
		stored(2, "https://a.org/c/b", true),
		stored(1, "https://a.org/c/a", true),
	}
	incoming := []domain.ChapterInfo{
		info(3, "https://a.org/c/b"), info(2, "https://a.org/c/a"), info(1, "https://a.org/c/new"),
	}
	res, err := mergeChapters(old, incoming, t1)
	if err != nil {
		t.Fatal(err)
	}
	m := byKey(t, res.Chapters)
	if !m["3"].Read || !m["2"].Read || m["1"].Read {
		t.Errorf("read state did not follow content: %+v", res.Chapters)
	}
	if res.New != 1 || len(res.Chapters) != 3 {
		t.Errorf("new=%d chapters=%d, want 1 and 3", res.New, len(res.Chapters))
	}
}

func TestMergeChapters_DomainChangeKeepsState(t *testing.T) {
	tests := []struct {
		name     string
		incoming domain.ChapterInfo
	}{
		{"same path on new host", info(5, "https://new-domain.net/c/5/")},
		{"new path, same number", info(5, "https://new-domain.net/series/x/chapter-5")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := []domain.Chapter{stored(5, "https://old.org/c/5", true)}
			res, err := mergeChapters(old, []domain.ChapterInfo{tt.incoming}, t1)
			if err != nil {
				t.Fatal(err)
			}
			c := res.Chapters[0]
			if len(res.Chapters) != 1 || !c.Read || c.URL != tt.incoming.URL || res.New != 0 {
				t.Errorf("chapters = %+v, new = %d", res.Chapters, res.New)
			}
		})
	}
}

func TestMergeChapters_DuplicatesAndAltURLs(t *testing.T) {
	old := []domain.Chapter{stored(7, "https://a.org/c/7", true)}
	in := info(7, "https://a.org/c/7")
	in.AltURLs = []string{"https://b.org/7"}
	res, err := mergeChapters(old, []domain.ChapterInfo{in, info(7, "https://c.org/7-again")}, t1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Chapters) != 1 || !res.Chapters[0].Read || len(res.Chapters[0].AltURLs) != 1 {
		t.Errorf("chapters = %+v", res.Chapters)
	}
}

func TestMergeChapters_ChaptersWithoutNumber(t *testing.T) {
	extra := fromInfo(domain.ChapterInfo{Label: "Afterword", URL: "https://a.org/afterword"})
	extra.Read = true
	old := []domain.Chapter{extra}
	tests := []struct {
		name string
		in   domain.ChapterInfo
	}{
		{"same label, new URL", domain.ChapterInfo{Label: "  afterword ", URL: "https://a.org/after-2"}},
		{"new label, same URL", domain.ChapterInfo{Label: "Afterword (Part 1)", URL: "https://a.org/afterword/"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := mergeChapters(old, []domain.ChapterInfo{tt.in}, t1)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Chapters) != 1 || !res.Chapters[0].Read || res.Chapters[0].Number != nil {
				t.Errorf("chapters = %+v", res.Chapters)
			}
		})
	}
}

func TestMergeChapters_RemovedReadChapterWhoseKeyIsReused(t *testing.T) {
	// Old ch 2 (read) disappears, and the URL of old ch 1 is now numbered 2.
	old := []domain.Chapter{stored(2, "https://a.org/c/x", true), stored(1, "https://a.org/c/y", false)}
	res, err := mergeChapters(old, []domain.ChapterInfo{info(2, "https://a.org/c/y")}, t1)
	if err != nil {
		t.Fatal(err)
	}
	m := byKey(t, res.Chapters)
	if len(res.Chapters) != 1 || !m["2"].Read || !m["2"].ReadAt.Equal(t0) {
		t.Errorf("read state of key 2 lost: %+v", res.Chapters)
	}
}

func TestMergeChapters_KeepsReleaseDateAndUnknownFields(t *testing.T) {
	old := stored(1, "https://a.org/c/1", false)
	old.ReleasedAt = domain.NewDate(2026, 10, 1)
	old.Extra = domain.Extra{"bookmark_page": json.RawMessage(`12`)}
	res, err := mergeChapters([]domain.Chapter{old}, []domain.ChapterInfo{info(1, "https://a.org/c/1")}, t1)
	if err != nil {
		t.Fatal(err)
	}
	c := res.Chapters[0]
	if c.ReleasedAt.String() != "2026-10-01" || string(c.Extra["bookmark_page"]) != "12" {
		t.Errorf("chapter = %+v", c)
	}
	in := info(1, "https://a.org/c/1")
	in.ReleasedAt = domain.NewDate(2026, 10, 2)
	res, _ = mergeChapters([]domain.Chapter{old}, []domain.ChapterInfo{in}, t1)
	if res.Chapters[0].ReleasedAt.String() != "2026-10-02" {
		t.Error("a release date from the source should replace the stored one")
	}
}

func TestMergeChapters_RefusesShrunkSource(t *testing.T) {
	var old []domain.Chapter
	var half []domain.ChapterInfo
	for i := range 12 {
		u := "https://a.org/c/" + string(rune('a'+i))
		old = append(old, stored(float64(i), u, true))
		if i < 5 {
			half = append(half, info(float64(i), u))
		}
	}
	tests := []struct {
		name     string
		stored   []domain.Chapter
		incoming []domain.ChapterInfo
		wantErr  bool
	}{
		{"empty source", old[:1], nil, true},
		{"less than half of a large list", old, half, true},
		{"small list may shrink", old[:4], half[:1], false},
		{"empty library accepts anything", nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mergeChapters(tt.stored, tt.incoming, t1)
			if tt.wantErr != errors.Is(err, ErrSourceShrank) {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestURLPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://a.org/c/1/", "/c/1"},
		{"https://b.net/c/1", "/c/1"},
		{"https://a.org/?p=12", "?p=12"},
		{"https://a.org", ""},
		{"%zz", ""},
	}
	for _, tt := range tests {
		if got := urlPath(tt.in); got != tt.want {
			t.Errorf("urlPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMergeChapters_DuplicateInSourceKeepsReadChapterItsURLPointsTo(t *testing.T) {
	old := []domain.Chapter{stored(1, "https://a.org/c/1", false), stored(2, "https://a.org/c/2", true)}
	// The source lists chapter 1 twice; the copy has chapter 2's URL.
	res, err := mergeChapters(old, []domain.ChapterInfo{info(1, "https://a.org/c/1"), info(1, "https://a.org/c/2")}, t1)
	if err != nil {
		t.Fatal(err)
	}
	m := byKey(t, res.Chapters)
	if c, ok := m["2"]; !ok || !c.Read || !c.ReadAt.Equal(t0) {
		t.Errorf("read chapter 2 lost: %+v", res.Chapters)
	}
	if res.New != 0 {
		t.Errorf("new = %d, want 0", res.New)
	}
}

func TestMergeChapters_KeepsTheURLWhenTheSourceLostIt(t *testing.T) {
	old := []domain.Chapter{stored(1, "https://a.org/c/1", true)}
	res, err := mergeChapters(old, []domain.ChapterInfo{info(1, "")}, t1)
	if err != nil {
		t.Fatal(err)
	}
	if c := res.Chapters[0]; c.URL != "https://a.org/c/1" || !c.Read {
		t.Errorf("chapter = %+v", c)
	}
}
