package extract

import (
	"errors"
	"net/url"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"https://Ravenscans.ORG/manga/x/?utm_source=tw&utm_medium=s#top", "https://ravenscans.org/manga/x/"},
		{" https://a.org/m?page=2&fbclid=abc ", "https://a.org/m?page=2"},
		{"http://a.org/m", "http://a.org/m"},
	}
	for _, tt := range tests {
		u, err := NormalizeURL(tt.in)
		if err != nil {
			t.Fatalf("%q: %v", tt.in, err)
		}
		if u.String() != tt.want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", tt.in, u, tt.want)
		}
	}
}

func TestNormalizeURL_RejectsInvalid(t *testing.T) {
	for _, in := range []string{"", "manhuaplus.org/manga/x", "javascript:alert(1)", "ftp://a.org", "%zz"} {
		if _, err := NormalizeURL(in); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("NormalizeURL(%q) err = %v, want ErrInvalidInput", in, err)
		}
	}
}

func TestCanonicalKey_IgnoresSchemeWWWAndTrailingSlash(t *testing.T) {
	a, _ := url.Parse("https://www.mgeko.cc/manga/grand-finale/")
	b, _ := url.Parse("http://mgeko.cc/manga/grand-finale")
	c, _ := url.Parse("https://mgeko.cc/manga/other")
	if CanonicalKey(a) != CanonicalKey(b) {
		t.Errorf("%q != %q", CanonicalKey(a), CanonicalKey(b))
	}
	if CanonicalKey(a) == CanonicalKey(c) {
		t.Error("different series share a key")
	}
	q, _ := url.Parse("https://a.org/s?id=1")
	if CanonicalKey(q) != "a.org/s?id=1" {
		t.Errorf("query dropped: %q", CanonicalKey(q))
	}
}

func TestDomain(t *testing.T) {
	u, _ := url.Parse("https://WWW.Mgeko.cc:443/x")
	if got := Domain(u); got != "mgeko.cc" {
		t.Errorf("Domain = %q", got)
	}
}

func TestResolveURL(t *testing.T) {
	base, _ := url.Parse("https://a.org/manga/x/")
	tests := []struct{ href, want string }{
		{"chapter-1/", "https://a.org/manga/x/chapter-1/"},
		{"/img/c.jpg", "https://a.org/img/c.jpg"},
		{"//cdn.a.org/c.jpg", "https://cdn.a.org/c.jpg"},
		{"https://b.org/y", "https://b.org/y"},
		{"#", ""},
		{"", ""},
		{"javascript:void(0)", ""},
		{"%zz", ""},
	}
	for _, tt := range tests {
		if got := ResolveURL(base, tt.href); got != tt.want {
			t.Errorf("ResolveURL(%q) = %q, want %q", tt.href, got, tt.want)
		}
	}
}
