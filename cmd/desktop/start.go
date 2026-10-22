package main

import (
	"html/template"
	"net/http"
)

// startHTML is the first page the window loads: it goes to the local
// library or, in connected mode, to the server, and says so when the
// server does not answer (no-cors: only reachability matters).
var startHTML = template.Must(template.New("start").Parse(`<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><title>LeftOff</title>
<style>body{font-family:system-ui,sans-serif;margin:3em auto;max-width:36em;padding:0 1em;color:#222;background:#f6f6f8}
@media (prefers-color-scheme:dark){body{color:#eee;background:#111317}a{color:#8ab4ff}}</style></head>
<body><p id="msg">{{.Opening}}</p><p id="alt" hidden><a href="{{.Local}}">{{.UseLocal}}</a></p>
<script>
const target = {{.Target}}, local = {{.Local}};
if (target === local) { location.replace(local); } else {
  const ctl = new AbortController(); setTimeout(() => ctl.abort(), 6000);
  fetch(target + "api/v1/health", {mode: "no-cors", signal: ctl.signal})
    .then(() => location.replace(target))
    .catch(() => { document.getElementById("msg").textContent = {{.Unreachable}}; document.getElementById("alt").hidden = false; });
}
</script></body></html>`))

var startTexts = map[string]map[string]string{
	"pt-BR": {"opening": "Abrindo o LeftOff…", "unreachable": "O servidor não respondeu: ",
		"local": "Usar a biblioteca deste computador"},
	"en": {"opening": "Opening LeftOff…", "unreachable": "The server did not answer: ",
		"local": "Use this computer’s library"},
}

// startPage serves startHTML for every request of the Wails asset server.
func startPage(local string, desk desktopSettings, lang string) http.Handler {
	target := local
	if desk.Mode == "connected" && desk.ServerURL != "" {
		target = desk.ServerURL + "/"
	}
	t := startTexts[lang]
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = startHTML.Execute(w, map[string]any{ // the client may be gone; nothing to do
			"Lang": lang, "Target": target, "Local": local, "Opening": t["opening"],
			"Unreachable": t["unreachable"] + target, "UseLocal": t["local"],
		})
	})
}
