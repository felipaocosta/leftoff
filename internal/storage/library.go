package storage

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// CurrentSchema is the library.json schema version written by this build.
const CurrentSchema = 3

// Library is the root of library.json (SPEC §6.2).
//
// A *Library returned by Store.Library is a shared read-only snapshot and
// must not be modified; use Store.Update to change data.
type Library struct {
	Schema    int             `json:"schema"`
	UpdatedAt time.Time       `json:"updated_at"`
	Series    []domain.Series `json:"series"`
	Extra     domain.Extra    `json:"-"`
}

// MarshalJSON encodes the library and its unknown fields.
func (l Library) MarshalJSON() ([]byte, error) {
	type plain Library
	return domain.MarshalWithExtra(plain(l), l.Extra)
}

// UnmarshalJSON decodes the library and keeps unknown fields in Extra.
func (l *Library) UnmarshalJSON(b []byte) error {
	type plain Library
	var p plain
	extra, err := domain.UnmarshalWithExtra(b, &p)
	if err != nil {
		return err
	}
	*l = Library(p)
	l.Extra = extra
	return nil
}

// SeriesByID returns the index of the series with the given id, or -1.
func (l *Library) SeriesByID(id string) int {
	for i := range l.Series {
		if l.Series[i].ID == id {
			return i
		}
	}
	return -1
}

func newLibrary() *Library {
	return &Library{Schema: CurrentSchema, Series: []domain.Series{}}
}

// encodeLibrary renders the library as indented JSON, as stored on disk.
func encodeLibrary(l *Library) ([]byte, error) {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode library: %w", err)
	}
	return append(b, '\n'), nil
}

// clone returns a deep copy of l (domain.Series.Clone), so a failed or
// running update never touches the published snapshot.
func (l *Library) clone() (*Library, error) {
	c := *l
	c.Extra = l.Extra.Clone()
	if l.Series != nil {
		c.Series = make([]domain.Series, len(l.Series))
		for i, s := range l.Series {
			c.Series[i] = s.Clone()
		}
	}
	return &c, nil
}
