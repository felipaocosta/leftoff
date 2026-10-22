package domain

import "time"

// External sources of official metadata (RF-57).
const (
	ExternalAniList      = "anilist"
	ExternalMangaUpdates = "mangaupdates"
)

// External links a series to its entry in a metadata database such as
// AniList or MangaUpdates (RF-57). A nil External means the series was
// never looked up; an External with an empty ID records a search that
// found nothing (retried later) or a link the user removed (Ignored).
type External struct {
	Source    string    `json:"source"` // ExternalAniList or ExternalMangaUpdates; "" when not found
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Status    Status    `json:"status"` // official status, normalized
	Genres    []string  `json:"genres"`
	Year      *int      `json:"year"`
	Synopsis  string    `json:"synopsis"`
	Ignored   bool      `json:"ignored"` // the user removed a wrong match: never look up again
	CheckedAt time.Time `json:"checked_at"`
	Extra     Extra     `json:"-"`
}

// Found reports whether the series is linked to an entry.
func (e *External) Found() bool { return e != nil && e.ID != "" && !e.Ignored }

// MarshalJSON encodes the link and its unknown fields.
func (e External) MarshalJSON() ([]byte, error) {
	type plain External
	return MarshalWithExtra(plain(e), e.Extra)
}

// UnmarshalJSON decodes the link and keeps unknown fields in Extra.
func (e *External) UnmarshalJSON(b []byte) error {
	type plain External
	var p plain
	extra, err := UnmarshalWithExtra(b, &p)
	if err != nil {
		return err
	}
	*e = External(p)
	e.Extra = extra
	return nil
}
