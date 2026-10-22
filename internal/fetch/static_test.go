package fetch

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
)

func TestStatic_ServesPagesByMethodAndURL(t *testing.T) {
	s := &Static{Pages: map[string][]byte{
		"https://a.org/x":      []byte("get"),
		"POST https://a.org/x": []byte("post"),
	}}
	ctx := context.Background()
	if r, err := Get(ctx, s, "https://a.org/x"); err != nil || string(r.Body) != "get" {
		t.Errorf("GET = %v, %v", r, err)
	}
	if r, err := s.Fetch(ctx, &Request{Method: http.MethodPost, URL: "https://a.org/x"}); err != nil || string(r.Body) != "post" {
		t.Errorf("POST = %v, %v", r, err)
	}
	if _, err := Get(ctx, s, "https://a.org/missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing: err = %v", err)
	}
	if len(s.Requests) != 3 {
		t.Errorf("requests = %v", s.Requests)
	}
}

func TestStatic_InvalidURL(t *testing.T) {
	s := &Static{Pages: map[string][]byte{"%zz": nil}}
	if _, err := Get(context.Background(), s, "%zz"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}
