package domain

// SeriesInfo is the series metadata produced by an extraction, before it is
// turned into a library Series.
type SeriesInfo struct {
	URL       string     `json:"url"` // canonical URL of the series page
	Title     string     `json:"title"`
	AltTitles []string   `json:"alt_titles"`
	CoverURL  string     `json:"cover_url"`
	Status    Status     `json:"status"`
	StatusRaw string     `json:"status_raw"`
	Synopsis  string     `json:"synopsis"`
	Type      SeriesType `json:"type"` // best guess; TypeOther when unknown
}

// ChapterInfo is a chapter as found on the source. Extractors return chapters
// in source order; that order is kept for chapters without a number.
type ChapterInfo struct {
	Number     *float64 `json:"number"`
	Label      string   `json:"label"`
	URL        string   `json:"url"`
	AltURLs    []string `json:"alt_urls"` // other scans of the same chapter
	ReleasedAt Date     `json:"released_at"`
}

// Key returns the stable chapter key for this chapter (see ChapterKey).
func (c ChapterInfo) Key() string { return ChapterKey(c.Number, c.Label) }
