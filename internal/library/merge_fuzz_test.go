package library

import (
	"fmt"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// fuzzChapters turns bytes into chapters from a tiny space of numbers,
// labels, paths and hosts, so collisions (renumbering, duplicates, host
// moves, chapters without a number) are common. Each chapter uses 2 bytes.
func fuzzChapters(b []byte) []domain.ChapterInfo {
	var out []domain.ChapterInfo
	for i := 0; i+1 < len(b); i += 2 {
		c := domain.ChapterInfo{
			Label: []string{"Chapter", "Extra", "Afterword"}[int(b[i]>>4)%3],
			URL:   fmt.Sprintf("https://%s/c/%d", []string{"a.org", "b.net"}[b[i+1]&1], (b[i+1]>>1)%6),
		}
		if n := b[i] % 8; n < 6 {
			c.Number = num(float64(n) / 2) // 0, 0.5, 1 … 2.5
		}
		out = append(out, c)
	}
	return out
}

// fuzzStored makes stored chapters from bytes; the first byte of each
// chapter's pair also decides whether it was read.
func fuzzStored(b []byte) []domain.Chapter {
	var out []domain.Chapter
	seen := map[string]bool{}
	for i, in := range fuzzChapters(b) {
		c := fromInfo(in)
		if seen[c.Key] {
			continue // a stored list never has duplicate keys
		}
		seen[c.Key] = true
		if b[2*i]&0x80 != 0 {
			at := t0
			c.Read, c.ReadAt = true, &at
		}
		out = append(out, c)
	}
	return out
}

func FuzzMergeChapters_NeverLosesReadState(f *testing.F) {
	f.Add([]byte{0x80, 0, 0x81, 2}, []byte{1, 0, 0, 2})          // renumbered
	f.Add([]byte{0x82, 4}, []byte{2, 5})                         // moved host
	f.Add([]byte{0x86, 0, 0x96, 2}, []byte{6, 0})                // no number
	f.Add([]byte{0, 0, 0x81, 2}, []byte{0, 4, 0, 2})             // duplicate in source
	f.Add([]byte{0x80, 0, 0x81, 2, 0x82, 4}, []byte{3, 6, 4, 8}) // all replaced
	f.Fuzz(func(t *testing.T, storedBytes, incomingBytes []byte) {
		old := fuzzStored(storedBytes)
		incoming := fuzzChapters(incomingBytes)
		res, err := mergeChapters(old, incoming, t1)
		if err != nil {
			if checkShrink(len(old), len(incoming)) == nil {
				t.Fatalf("unexpected error: %v", err)
			}
			return // refused: nothing changes
		}
		out := map[string]domain.Chapter{}
		for _, c := range res.Chapters {
			if _, dup := out[c.Key]; dup {
				t.Fatalf("duplicate key %q", c.Key)
			}
			out[c.Key] = c
		}
		for _, in := range incoming {
			if _, ok := out[in.Key()]; !ok {
				t.Fatalf("source chapter %q missing", in.Key())
			}
		}
		for _, s := range old {
			if s.Read && !keptRead(s, res.Chapters) {
				t.Fatalf("read chapter %+v lost in %+v", s, res.Chapters)
			}
		}
	})
}

// keptRead reports whether the read state of s survived: a read chapter
// with its key (kept, or moved to the live chapter with that key) or its
// URL path (renumbered) is in out, with the read time.
func keptRead(s domain.Chapter, out []domain.Chapter) bool {
	for _, c := range out {
		if c.Read && c.ReadAt != nil && (c.Key == s.Key || urlPath(c.URL) == urlPath(s.URL)) {
			return true
		}
	}
	return false
}
