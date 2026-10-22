package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSeries_PreservesUnknownFieldsOnRoundTrip(t *testing.T) {
	in := `{
		"id": "01J", "title": "X", "future_field": {"a": 1},
		"source": {"url": "https://a.org/x", "domain": "a.org", "extra_src": true},
		"cover": {"remote_url": "", "local": "", "blur": "abc"},
		"chapters": [{"key": "1", "number": 1, "label": "Ch 1", "read": true, "scanlator": "Team"}]
	}`
	var s Series
	if err := json.Unmarshal([]byte(in), &s); err != nil {
		t.Fatal(err)
	}
	if s.Title != "X" || !s.Chapters[0].Read {
		t.Fatalf("known fields not decoded: %+v", s)
	}
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"future_field":{"a":1}`, `"extra_src":true`, `"blur":"abc"`, `"scanlator":"Team"`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output lacks %s:\n%s", want, out)
		}
	}
	if strings.Count(string(out), `"title"`) != 1 {
		t.Errorf("known field duplicated:\n%s", out)
	}
}

func TestUnmarshalWithExtra_KnownKeysAreCaseInsensitive(t *testing.T) {
	var c Cover
	if err := json.Unmarshal([]byte(`{"Remote_URL": "u"}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.RemoteURL != "u" || c.Extra != nil {
		t.Errorf("got %+v, want RemoteURL=u and no extra", c)
	}
}

func TestMarshalWithExtra(t *testing.T) {
	type empty struct{}
	type one struct {
		A int `json:"a"`
	}
	tests := []struct {
		name  string
		v     any
		extra Extra
		want  string
	}{
		{"no extra", one{A: 1}, nil, `{"a":1}`},
		{"extra sorted after known", one{A: 1}, Extra{"z": json.RawMessage(`2`), "b": json.RawMessage(`"x"`)}, `{"a":1,"b":"x","z":2}`},
		{"empty struct", empty{}, Extra{"k": json.RawMessage(`true`)}, `{"k":true}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MarshalWithExtra(tt.v, tt.extra)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestUnmarshalJSON_InvalidInputFails(t *testing.T) {
	targets := []any{&Series{}, &Chapter{}, &Source{}, &Cover{}}
	for _, v := range targets {
		if err := json.Unmarshal([]byte(`{"title": 5, "key": 5, "url": 5, "local": 5}`), v); err == nil {
			t.Errorf("%T: expected type error", v)
		}
	}
}

func TestMarshalWithExtra_UnsupportedValueFails(t *testing.T) {
	if _, err := MarshalWithExtra(make(chan int), nil); err == nil {
		t.Error("expected error for channel")
	}
}

func TestUnmarshalWithExtra_FastAndSlowPathsAgree(t *testing.T) {
	type plain struct {
		A int    `json:"a"`
		B string `json:"b"`
	}
	tests := []struct {
		name, in string
		want     plain
		extra    []string
	}{
		{"only known fields", `{"a":1,"b":"x"}`, plain{1, "x"}, nil},
		{"unknown field kept", `{"a":1,"z":[1,2]}`, plain{A: 1}, []string{"z"}},
		{"known field in other case", `{"A":2}`, plain{A: 2}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got plain
			extra, err := UnmarshalWithExtra([]byte(tt.in), &got)
			if err != nil || got != tt.want || len(extra) != len(tt.extra) {
				t.Fatalf("got %+v extra %v err %v", got, extra, err)
			}
			for _, k := range tt.extra {
				if _, ok := extra[k]; !ok {
					t.Errorf("extra lacks %q", k)
				}
			}
		})
	}
	var p plain
	if _, err := UnmarshalWithExtra([]byte(`{"a":1} {"a":2}`), &p); err == nil {
		t.Error("trailing data must be an error")
	}
}
