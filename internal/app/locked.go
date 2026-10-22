package app

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// lockedTexts are the "library open elsewhere" page texts, by language.
var lockedTexts = map[string]map[string]string{
	"pt-BR": {
		"text":     "A biblioteca já está aberta em outro LeftOff. Dois apps na mesma pasta apagariam as mudanças um do outro.",
		"button":   "Encerrar o outro e abrir este",
		"working":  "Encerrando o outro app…",
		"done":     "Pronto, abrindo…",
		"failed":   "Não foi possível encerrar o outro app: ",
		"or_close": "Ou feche o outro app e abra este de novo.",
	},
	"en": {
		"text":     "The library is already open in another LeftOff. Two apps on one folder would erase each other’s changes.",
		"button":   "Close the other one and open this",
		"working":  "Closing the other app…",
		"done":     "Done, opening…",
		"failed":   "The other app could not be closed: ",
		"or_close": "Or close the other app and open this one again.",
	},
}

var lockedHTML = template.Must(template.New("locked").Parse(`<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><title>LeftOff</title>
<style>body{font-family:system-ui,sans-serif;margin:3em auto;max-width:36em;padding:0 1em;color:#222;background:#f6f6f8}
button{font:inherit;padding:.5em 1em;border-radius:6px;border:1px solid #3b5bdb;background:#3b5bdb;color:#fff;cursor:pointer}
button:disabled{opacity:.6}small{color:#777}
@media (prefers-color-scheme:dark){body{color:#eee;background:#111317}}</style></head>
<body><h1>LeftOff</h1><p>{{.T.text}}</p>
<p><button id="go">{{.T.button}}</button></p><p id="msg" role="status"></p>
<p><small>{{.T.or_close}}<br>{{.Detail}}</small></p>
<script>
const btn = document.getElementById("go"), msg = document.getElementById("msg");
btn.onclick = async () => {
  btn.disabled = true; msg.textContent = {{.T.working}};
  try {
    const r = await fetch("takeover", {method: "POST"});
    if (!r.ok) throw new Error(await r.text());
    msg.textContent = {{.T.done}};
  } catch (e) { msg.textContent = {{.T.failed}} + e.message; btn.disabled = false; }
};
</script></body></html>`))

// LockedPage is the "library open elsewhere" page (desktop window, or the
// browser in app mode) with a button that frees the folder (Takeover) and
// then calls opened, which starts this app.
func LockedPage(lang, detail, dataDir string, opened func()) http.Handler {
	t, ok := lockedTexts[lang]
	if !ok {
		lang, t = "en", lockedTexts["en"]
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /takeover", func(w http.ResponseWriter, r *http.Request) {
		if err := Takeover(r.Context(), dataDir); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		go opened() // after the answer
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = lockedHTML.Execute(w, map[string]any{"Lang": lang, "T": t, "Detail": detail}) // the client may be gone
	})
	return mux
}

// SystemLanguage picks the UI language from the environment (LC_ALL,
// LC_MESSAGES, LANG): pt-BR for Portuguese, en otherwise.
func SystemLanguage(getenv func(string) string) string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := getenv(k); v != "" {
			if strings.HasPrefix(strings.ToLower(v), "pt") {
				return "pt-BR"
			}
			return "en"
		}
	}
	return "en"
}

// ServeLockedPage shows LockedPage on a free local port until the user
// frees the folder (true) or ctx ends (false); open shows the page.
func ServeLockedPage(ctx context.Context, lang, detail, dataDir string, open func(url string) error) (bool, error) {
	freed := make(chan struct{})
	ln, err := listen(ctx, "127.0.0.1", 0, false, slog.New(slog.DiscardHandler))
	if err != nil {
		return false, err
	}
	srv := &http.Server{Handler: LockedPage(lang, detail, dataDir, func() { close(freed) }), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }() // closed below
	defer func() { _ = srv.Close() }()
	if err := open("http://" + ln.Addr().String() + "/"); err != nil {
		return false, err
	}
	select {
	case <-freed:
		return true, nil
	case <-ctx.Done():
		return false, nil
	}
}
