package extract

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// movingSite answers every request as if it had been redirected to newHost.
type movingSite struct {
	newHost string
	body    string
}

func (m movingSite) Fetch(_ context.Context, req *fetch.Request) (*fetch.Response, error) {
	u, _ := url.Parse(req.URL)
	final := *u
	final.Host = m.newHost
	return &fetch.Response{URL: &final, StatusCode: http.StatusOK, Body: []byte(m.body)}, nil
}

func TestExtract_AdapterReportsTheNewDomainAfterARedirect(t *testing.T) {
	a := fetchingAdapter{&fakeAdapter{name: "x", host: "site.org", title: "T"}}
	ex := New(movingSite{newHost: "site.net"}, Options{Adapters: []Adapter{a}})
	res, err := ex.Extract(context.Background(), "https://site.org/manga/x/")
	if err != nil {
		t.Fatal(err)
	}
	if res.Series.URL != "https://site.net/manga/x/" {
		t.Errorf("series url = %q, want the redirected domain", res.Series.URL)
	}
}

func TestExtract_NoRedirectKeepsTheURL(t *testing.T) {
	a := fetchingAdapter{&fakeAdapter{name: "x", host: "site.org", title: "T"}}
	ex := New(movingSite{newHost: "site.org"}, Options{Adapters: []Adapter{a}})
	res, err := ex.Extract(context.Background(), "https://site.org/manga/x/")
	if err != nil || res.Series.URL != "https://site.org/manga/x/" {
		t.Errorf("url = %q, err = %v", res.Series.URL, err)
	}
}

// fetchingAdapter makes a fakeAdapter load its page through the fetcher.
type fetchingAdapter struct{ *fakeAdapter }

func (f fetchingAdapter) FetchSeries(ctx context.Context, fe fetch.Fetcher, u *url.URL) (*domain.SeriesInfo, error) {
	if _, err := fetch.Get(ctx, fe, u.String()); err != nil {
		return nil, err
	}
	info, err := f.fakeAdapter.FetchSeries(ctx, fe, u)
	if err == nil {
		info.URL = u.String()
	}
	return info, err
}
