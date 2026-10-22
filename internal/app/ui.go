package app

import (
	"io/fs"
	"net/http"
	"strings"
)

// contentSecurityPolicy keeps the UI to its own scripts. Remote images are
// allowed because a series without a local cover shows the remote one.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' https: data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'"

// uiNotice is shown when the binary was compiled without building web/.
const uiNotice = "LeftOff: UI not built. Run `npm ci && npm run build` in web/ and rebuild the server. " +
	"The API is available at /api/v1.\n"

// newRootHandler routes /api/ to the API and everything else to the UI.
func newRootHandler(apiHandler http.Handler, ui fs.FS, built bool) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/", uiHandler(ui, built))
	return mux
}

func uiHandler(ui fs.FS, built bool) http.Handler {
	files := http.FileServerFS(ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if !built {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(uiNotice)) // the client may be gone; nothing left to do
			return
		}
		if strings.HasSuffix(r.URL.Path, ".webmanifest") {
			w.Header().Set("Content-Type", "application/manifest+json")
		}
		// Hashed assets never change; index.html must always be revalidated.
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}
