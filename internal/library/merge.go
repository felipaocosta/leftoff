package library

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// ErrSourceShrank means a refresh returned far fewer chapters than the
// library has, which usually is a broken or partial page. The merge is
// refused so nothing is dropped; the series gets a check error instead.
var ErrSourceShrank = errors.New("source returned suspiciously few chapters")

// shrinkGuardMin is the library size from which a source returning less
// than half of the known chapters is treated as broken.
const shrinkGuardMin = 10

// mergeResult is the outcome of merging a fresh chapter list.
type mergeResult struct {
	Chapters []domain.Chapter
	New      int // chapters not seen before
}

// mergeChapters combines the stored chapters with the list just extracted
// from the source, never losing read state (RF-14, RULES §7):
//
//   - each incoming chapter is matched to a stored one first by URL path
//     (survives renumbering and host changes), then by stable key (survives
//     path changes); a stored chapter is matched at most once;
//   - a match keeps read, read_at, discovered_at and unknown fields, and
//     takes number, key, label, URL and alt URLs from the source; the
//     release date is only replaced when the source has one;
//   - an unmatched incoming chapter is new and gets discovered_at = now;
//   - a stored chapter missing from the source is kept only if it was read;
//     if its key is now used by a live chapter, its read state moves there.
//
// Output order is the source order, followed by the kept removed chapters.
func mergeChapters(stored []domain.Chapter, incoming []domain.ChapterInfo, now time.Time) (mergeResult, error) {
	if err := checkShrink(len(stored), len(incoming)); err != nil {
		return mergeResult{}, err
	}
	byPath := map[string]int{}
	byKey := map[string]int{}
	for i, c := range stored {
		if p := urlPath(c.URL); p != "" {
			byPath[p] = i
		}
		byKey[c.Key] = i
	}

	used := make([]bool, len(stored))
	out := make([]domain.Chapter, 0, len(incoming))
	outIndex := map[string]int{}
	res := mergeResult{}
	for _, in := range incoming {
		ch := fromInfo(in)
		if _, dup := outIndex[ch.Key]; dup {
			// Extract already dedupes; keep the first just in case. The
			// duplicate matches nothing, so the stored chapter its URL
			// points to is still handled below as removed (kept if read).
			continue
		}
		if i, ok := match(in, ch.Key, byPath, byKey, used); ok {
			used[i] = true
			ch = carryState(stored[i], ch)
		} else {
			discovered := now
			ch.DiscoveredAt = &discovered
			res.New++
		}
		outIndex[ch.Key] = len(out)
		out = append(out, ch)
	}

	for i, old := range stored {
		if used[i] || !old.Read {
			continue
		}
		if j, taken := outIndex[old.Key]; taken {
			out[j] = mergeReadState(out[j], old)
			continue
		}
		outIndex[old.Key] = len(out)
		out = append(out, old)
	}
	res.Chapters = out
	return res, nil
}

func checkShrink(stored, incoming int) error {
	if stored > 0 && incoming == 0 {
		return fmt.Errorf("%w: 0 of %d", ErrSourceShrank, stored)
	}
	if stored >= shrinkGuardMin && incoming*2 < stored {
		return fmt.Errorf("%w: %d of %d", ErrSourceShrank, incoming, stored)
	}
	return nil
}

// match finds the stored chapter for an incoming one: URL path first, then
// key, skipping stored chapters already matched.
func match(in domain.ChapterInfo, key string, byPath, byKey map[string]int, used []bool) (int, bool) {
	if i, ok := byPath[urlPath(in.URL)]; ok && urlPath(in.URL) != "" && !used[i] {
		return i, true
	}
	if i, ok := byKey[key]; ok && !used[i] {
		return i, true
	}
	return 0, false
}

// fromInfo builds a fresh, unread chapter from extraction data.
func fromInfo(in domain.ChapterInfo) domain.Chapter {
	alt := in.AltURLs
	if alt == nil {
		alt = []string{}
	}
	return domain.Chapter{
		Key:        in.Key(),
		Number:     in.Number,
		Label:      in.Label,
		URL:        in.URL,
		AltURLs:    alt,
		ReleasedAt: in.ReleasedAt,
	}
}

// carryState copies the user and history state of old into fresh.
func carryState(old, fresh domain.Chapter) domain.Chapter {
	fresh.Read = old.Read
	fresh.ReadAt = old.ReadAt
	fresh.DiscoveredAt = old.DiscoveredAt
	fresh.Extra = old.Extra
	if fresh.URL == "" { // a source without the link this time keeps the known one
		fresh.URL = old.URL
	}
	if fresh.ReleasedAt.IsZero() {
		fresh.ReleasedAt = old.ReleasedAt
	}
	return fresh
}

// mergeReadState marks live as read when old (same key) was read, keeping
// the earliest read time.
func mergeReadState(live, old domain.Chapter) domain.Chapter {
	if !live.Read {
		live.Read, live.ReadAt = true, old.ReadAt
		return live
	}
	if old.ReadAt != nil && (live.ReadAt == nil || old.ReadAt.Before(*live.ReadAt)) {
		live.ReadAt = old.ReadAt
	}
	return live
}

// urlPath is the host-independent part of a chapter URL (path + query),
// so a move to a new domain keeps matching.
func urlPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Path == "" {
		return ""
	}
	p := strings.TrimSuffix(u.EscapedPath(), "/")
	if u.RawQuery != "" {
		p += "?" + u.RawQuery
	}
	return p
}
