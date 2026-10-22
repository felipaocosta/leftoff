package domain

import (
	"maps"
	"slices"
)

// Clone returns a deep copy of s: changing the copy never changes s. The
// storage snapshot is copied this way before every write (it used to be a
// JSON round trip, seconds with 500 series). TestClone_SharesNoMemory
// walks every field, so a new field that is not copied here fails it.
func (s Series) Clone() Series {
	c := s
	c.AltTitles = slices.Clone(s.AltTitles)
	c.Tags = slices.Clone(s.Tags)
	c.Source = s.Source.clone()
	c.Cover = s.Cover.clone()
	c.LatestChapter = clonePtr(s.LatestChapter)
	c.TotalChapters = clonePtr(s.TotalChapters)
	c.LastReadChapter = clonePtr(s.LastReadChapter)
	c.LastCheckedAt = clonePtr(s.LastCheckedAt)
	c.LastNewChapterAt = clonePtr(s.LastNewChapterAt)
	c.SeenLatestChapter = clonePtr(s.SeenLatestChapter)
	c.CheckError = clonePtr(s.CheckError)
	c.ReleaseIntervalDays = clonePtr(s.ReleaseIntervalDays)
	if s.External != nil {
		e := s.External.clone()
		c.External = &e
	}
	if s.Chapters != nil {
		c.Chapters = make([]Chapter, len(s.Chapters))
		for i, ch := range s.Chapters {
			c.Chapters[i] = ch.clone()
		}
	}
	c.Extra = s.Extra.Clone()
	return c
}

func (c Chapter) clone() Chapter {
	c.Number = clonePtr(c.Number)
	c.AltURLs = slices.Clone(c.AltURLs)
	c.DiscoveredAt = clonePtr(c.DiscoveredAt)
	c.ReadAt = clonePtr(c.ReadAt)
	c.Extra = c.Extra.Clone()
	return c
}

func (s Source) clone() Source {
	s.Mirrors = slices.Clone(s.Mirrors)
	s.Extra = s.Extra.Clone()
	return s
}

func (c Cover) clone() Cover {
	c.UpdatedAt = clonePtr(c.UpdatedAt)
	c.Extra = c.Extra.Clone()
	return c
}

func (e External) clone() External {
	e.Genres = slices.Clone(e.Genres)
	e.Year = clonePtr(e.Year)
	e.Extra = e.Extra.Clone()
	return e
}

// Clone copies the map and each raw value.
func (x Extra) Clone() Extra {
	if x == nil {
		return nil
	}
	c := maps.Clone(x)
	for k, v := range c {
		c[k] = slices.Clone(v)
	}
	return c
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
