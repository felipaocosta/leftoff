package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func page(t *testing.T, desk desktopSettings, lang string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	startPage("http://127.0.0.1:4000/", desk, lang).ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), "GET", "/anything", nil))
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content type %q", ct)
	}
	return rec.Body.String()
}

func TestStartPage(t *testing.T) {
	local := page(t, desktopSettings{Mode: "standalone"}, "pt-BR")
	if !strings.Contains(local, `const target = "http://127.0.0.1:4000/", local = "http://127.0.0.1:4000/"`) ||
		!strings.Contains(local, "Abrindo o LeftOff") {
		t.Errorf("standalone page:\n%s", local)
	}
	remote := page(t, desktopSettings{Mode: "connected", ServerURL: "http://nas:8080"}, "en")
	if !strings.Contains(remote, `const target = "http://nas:8080/"`) || !strings.Contains(remote, "The server did not answer: http://nas:8080/") ||
		!strings.Contains(remote, `<a href="http://127.0.0.1:4000/">Use this computer’s library</a>`) {
		t.Errorf("connected page:\n%s", remote)
	}
	if p := page(t, desktopSettings{Mode: "connected"}, "en"); !strings.Contains(p, `const target = "http://127.0.0.1:4000/"`) {
		t.Error("connected without a URL opens the local library")
	}
}

func TestStartPage_EscapesTheServerURL(t *testing.T) {
	p := page(t, desktopSettings{Mode: "connected", ServerURL: `http://x/"</script><script>alert(1)`}, "en")
	if strings.Contains(p, "<script>alert(1)") {
		t.Errorf("unescaped URL:\n%s", p)
	}
}

func TestMenuLanguage(t *testing.T) {
	for _, tt := range []struct{ lcAll, lang, want string }{
		{"", "pt_BR.UTF-8", "pt-BR"},
		{"", "en_US.UTF-8", "en"},
		{"C", "pt_BR.UTF-8", "en"}, // LC_ALL wins
		{"", "", "pt-BR"},
	} {
		t.Setenv("LC_ALL", tt.lcAll)
		t.Setenv("LC_MESSAGES", "")
		t.Setenv("LANG", tt.lang)
		if got := menuLanguage(); got != tt.want {
			t.Errorf("LC_ALL=%q LANG=%q: %s, want %s", tt.lcAll, tt.lang, got, tt.want)
		}
	}
	for lang := range menuTexts {
		if len(menuTexts[lang]) != len(menuTexts["pt-BR"]) || len(startTexts[lang]) != len(startTexts["pt-BR"]) {
			t.Errorf("%s texts are incomplete", lang)
		}
	}
}
