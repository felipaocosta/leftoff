package domain

import "time"

// SeriesType is the kind of work (RF-09).
type SeriesType string

// Known series types.
const (
	TypeManga  SeriesType = "manga"
	TypeManhwa SeriesType = "manhwa"
	TypeManhua SeriesType = "manhua"
	TypeNovel  SeriesType = "novel"
	TypeOther  SeriesType = "other"
)

// Valid reports whether t is one of the known types.
func (t SeriesType) Valid() bool {
	switch t {
	case TypeManga, TypeManhwa, TypeManhua, TypeNovel, TypeOther:
		return true
	}
	return false
}

// Category is the user's reading list for a series (RF-07).
type Category string

// Known categories.
const (
	CategoryReading    Category = "reading"
	CategoryPlanToRead Category = "plan_to_read"
	CategoryOnHold     Category = "on_hold"
	CategoryDropped    Category = "dropped"
	CategoryCompleted  Category = "completed"
)

// Valid reports whether c is one of the known categories.
func (c Category) Valid() bool {
	switch c {
	case CategoryReading, CategoryPlanToRead, CategoryOnHold, CategoryDropped, CategoryCompleted:
		return true
	}
	return false
}

// Source describes where a series is extracted from.
type Source struct {
	URL     string   `json:"url"`
	Domain  string   `json:"domain"`
	Method  string   `json:"method"`
	Mirrors []string `json:"mirrors"`
	Extra   Extra    `json:"-"`
}

// Cover points to the remote cover image and its local copy. Custom means
// the user uploaded the local image, so refreshes must not replace it.
//
// UpdatedAt changes whenever the local file is replaced; the UI uses it as a
// cache-busting version, since the file keeps the same URL.
type Cover struct {
	RemoteURL string     `json:"remote_url"`
	Local     string     `json:"local"`
	Custom    bool       `json:"custom"`
	UpdatedAt *time.Time `json:"updated_at"`
	Extra     Extra      `json:"-"`
}

// CheckError records the last failed check of a series (RF-24).
// Failures counts consecutive failures and feeds the scheduler backoff (§9).
type CheckError struct {
	// Code is the kind of failure (Check* below), so the UI can explain
	// it in the user's language; "" is any other failure (schema 3).
	Code     string    `json:"code"`
	Message  string    `json:"message"`
	At       time.Time `json:"at"`
	Failures int       `json:"failures"`
}

// Check error codes; they match the API error codes (SPEC §7).
const (
	CheckNeedsBrowser  = "needs_browser"         // a JavaScript page and no browser to render it
	CheckNoBrowser     = "no_browser"            // an anti-bot check and no browser to pass it
	CheckBlocked       = "blocked"               // an anti-bot check that failed
	CheckSourceChanged = "source_changed"        // the chapter list shrank or vanished
	CheckIncomplete    = "extraction_incomplete" // the page lacks the title
	CheckNotFound      = "not_found"             // the page is gone
	CheckTimeout       = "timeout"
	CheckUnreachable   = "upstream_error" // the site is down or answered with an error
)

// Series is a tracked work as stored in library.json (SPEC §6.2).
type Series struct {
	ID                string      `json:"id"`
	Type              SeriesType  `json:"type"`
	Title             string      `json:"title"`
	AltTitles         []string    `json:"alt_titles"`
	Source            Source      `json:"source"`
	Cover             Cover       `json:"cover"`
	Status            Status      `json:"status"`
	StatusRaw         string      `json:"status_raw"`
	Synopsis          string      `json:"synopsis"`
	Tags              []string    `json:"tags"`
	Category          Category    `json:"category"`
	Notes             string      `json:"notes"`
	LatestChapter     *float64    `json:"latest_chapter"`
	TotalChapters     *int        `json:"total_chapters"`
	LastReadChapter   *float64    `json:"last_read_chapter"`
	AddedAt           time.Time   `json:"added_at"`
	LastCheckedAt     *time.Time  `json:"last_checked_at"`
	LastNewChapterAt  *time.Time  `json:"last_new_chapter_at"`
	SeenLatestChapter *float64    `json:"seen_latest_chapter"`
	CheckError        *CheckError `json:"check_error"`
	// External is the series' entry in AniList or MangaUpdates (RF-57,
	// schema 2); nil until it is looked up.
	External *External `json:"external"`
	// ReleaseIntervalDays and NextReleaseEstimate are derived from the
	// chapters' release dates (RF-27); both are null when unknown.
	ReleaseIntervalDays *float64  `json:"release_interval_days"`
	NextReleaseEstimate Date      `json:"next_release_estimate"`
	Chapters            []Chapter `json:"chapters"`
	Extra               Extra     `json:"-"`
}

// RecomputeDerived refreshes the fields derived from Chapters (SPEC §5.6 and
// §6.2): LatestChapter is the highest number, LastReadChapter the highest
// read number, TotalChapters is only set when the series is completed, and
// the release interval and next release estimate follow the release dates.
func (s *Series) RecomputeDerived() {
	s.LatestChapter = nil
	s.LastReadChapter = nil
	for _, ch := range s.Chapters {
		if ch.Number == nil {
			continue
		}
		s.LatestChapter = maxNumber(s.LatestChapter, *ch.Number)
		if ch.Read {
			s.LastReadChapter = maxNumber(s.LastReadChapter, *ch.Number)
		}
	}
	s.TotalChapters = nil
	if s.Status == StatusCompleted {
		n := len(s.Chapters)
		s.TotalChapters = &n
	}
	s.recomputeRelease()
}

// HasNew reports whether chapters were released since the user last
// cleared the "new" flag (RF-23).
func (s *Series) HasNew() bool {
	if s.LatestChapter == nil {
		return false
	}
	return s.SeenLatestChapter == nil || *s.LatestChapter > *s.SeenLatestChapter
}

// UnreadCount returns how many chapters are not marked as read.
func (s *Series) UnreadCount() int {
	n := 0
	for _, ch := range s.Chapters {
		if !ch.Read {
			n++
		}
	}
	return n
}

func maxNumber(cur *float64, v float64) *float64 {
	if cur == nil || v > *cur {
		return &v
	}
	return cur
}
