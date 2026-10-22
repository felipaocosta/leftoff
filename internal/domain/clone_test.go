package domain

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// filled returns a series where every pointer, slice and map is set, so a
// copy that forgets one shows up.
func filled(t *testing.T) Series {
	t.Helper()
	n, at, y := 3.5, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), 2020
	x := Extra{"future": json.RawMessage(`{"a":1}`)}
	s := Series{
		ID: "1", Title: "T", AltTitles: []string{"A"}, Tags: []string{"t"},
		Source:        Source{URL: "https://a.org/x", Mirrors: []string{"https://b.org/x"}, Extra: x.Clone()},
		Cover:         Cover{UpdatedAt: &at, Extra: x.Clone()},
		LatestChapter: &n, TotalChapters: new(int), LastReadChapter: &n, LastCheckedAt: &at,
		LastNewChapterAt: &at, SeenLatestChapter: &n, CheckError: &CheckError{Message: "m"},
		ReleaseIntervalDays: &n,
		External:            &External{Genres: []string{"Action"}, Year: &y, Extra: x.Clone()},
		Chapters: []Chapter{{Key: "3.5", Number: &n, AltURLs: []string{"u"}, DiscoveredAt: &at, ReadAt: &at,
			Read: true, ReleasedAt: NewDate(2026, 10, 1), Extra: x.Clone()}},
		Extra: x.Clone(),
	}
	return s
}

func TestClone_IsEqual(t *testing.T) {
	s := filled(t)
	if c := s.Clone(); !reflect.DeepEqual(s, c) {
		t.Errorf("clone differs:\n%+v\n%+v", s, c)
	}
	if c := (Series{}).Clone(); !reflect.DeepEqual(Series{}, c) {
		t.Errorf("zero clone = %+v", c)
	}
}

// TestClone_SharesNoMemory walks every field of the copy: no pointer,
// slice element or map may be the original's. A field added to Series
// (or its parts) without a line in Clone fails here.
func TestClone_SharesNoMemory(t *testing.T) {
	s := filled(t)
	c := s.Clone()
	noShared(t, "Series", reflect.ValueOf(&s).Elem(), reflect.ValueOf(&c).Elem())
}

func noShared(t *testing.T, path string, a, b reflect.Value) {
	t.Helper()
	switch a.Kind() {
	case reflect.Pointer:
		if a.IsNil() {
			return
		}
		if a.Pointer() == b.Pointer() {
			t.Errorf("%s: the copy shares the pointer", path)
			return
		}
		noShared(t, path, a.Elem(), b.Elem())
	case reflect.Slice:
		if a.Len() == 0 {
			return
		}
		if a.Pointer() == b.Pointer() {
			t.Errorf("%s: the copy shares the slice", path)
			return
		}
		for i := range a.Len() {
			noShared(t, path+"[]", a.Index(i), b.Index(i))
		}
	case reflect.Map:
		if a.Len() == 0 {
			return
		}
		if a.Pointer() == b.Pointer() {
			t.Errorf("%s: the copy shares the map", path)
			return
		}
		for _, k := range a.MapKeys() {
			noShared(t, path+"["+k.String()+"]", a.MapIndex(k), b.MapIndex(k))
		}
	case reflect.Struct:
		for i := range a.NumField() {
			if a.Type().Field(i).IsExported() {
				noShared(t, path+"."+a.Type().Field(i).Name, a.Field(i), b.Field(i))
			}
		}
	}
}

func TestClone_FilledSetsEveryReferenceField(t *testing.T) {
	// The check above only proves something if the sample has every
	// pointer, slice and map set: a nil one would pass trivially.
	s := filled(t)
	empty(t, "Series", reflect.ValueOf(s))
}

func empty(t *testing.T, path string, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map:
		if v.IsNil() || (v.Kind() != reflect.Pointer && v.Len() == 0) {
			t.Errorf("%s is empty in the sample: set it so the clone test covers it", path)
			return
		}
		if v.Kind() == reflect.Pointer {
			empty(t, path, v.Elem())
		}
		if v.Kind() == reflect.Slice {
			empty(t, path+"[]", v.Index(0))
		}
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) || v.Type() == reflect.TypeOf(Date{}) {
			return
		}
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				empty(t, path+"."+v.Type().Field(i).Name, v.Field(i))
			}
		}
	}
}
