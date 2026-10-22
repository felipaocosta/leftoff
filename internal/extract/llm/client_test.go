package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_SendsSchemaAndReadsReply(t *testing.T) {
	var got chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"a\":1}"}}]}`))
	}))
	defer srv.Close()

	for _, base := range []string{srv.URL, srv.URL + "/v1/", srv.URL + "/"} {
		c := NewClient(base, "qwen", srv.Client())
		reply, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, draftSchema)
		if err != nil || reply != `{"a":1}` {
			t.Fatalf("%s: reply %q, err %v", base, reply, err)
		}
	}
	if got.Model != "qwen" || got.Stream || got.ResponseFormat == nil ||
		got.ResponseFormat.Type != "json_schema" || !json.Valid(got.ResponseFormat.JSONSchema.Schema) {
		t.Errorf("request = %+v", got)
	}
}

func TestClient_Errors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"openai error", 404, `{"error":{"message":"model not found"}}`, "model not found"},
		{"string error", 400, `{"error":"context too long"}`, "context too long"},
		{"not json", 502, `<html>`, "HTTP 502"},
		{"bad status", 500, `{}`, "HTTP 500"},
		{"no choices", 200, `{"choices":[]}`, "empty reply"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			_, err := NewClient(srv.URL, "m", srv.Client()).Complete(context.Background(), nil, draftSchema)
			if !errors.Is(err, ErrModel) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want ErrModel with %q", err, tt.want)
			}
		})
	}
}

func TestClient_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	_, err := NewClient(srv.URL, "m", nil).Complete(context.Background(), nil, draftSchema)
	if !errors.Is(err, ErrModel) {
		t.Errorf("err = %v, want ErrModel", err)
	}
}
