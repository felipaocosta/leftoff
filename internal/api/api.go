// Package api exposes the library over HTTP/JSON under /api/v1 (SPEC §7).
//
// It is a thin layer: handlers decode the request, call library and encode
// the response; errors are mapped to HTTP status codes in one place. It
// does NOT contain business rules nor touch storage directly.
package api

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/library"
	"github.com/felipaocosta/leftoff/internal/scheduler"
)

// Service is the part of *library.Service the API uses.
type Service interface {
	Preview(ctx context.Context, rawURL string) (*extract.Result, error)
	Find(q library.Query) ([]domain.Series, error)
	Sources() []library.SourceCount
	Get(id string) (domain.Series, error)
	Add(ctx context.Context, rawURL string, o library.Overrides) (domain.Series, error)
	Edit(ctx context.Context, id string, p library.Patch) (domain.Series, error)
	Remove(ctx context.Context, id string) error
	Refresh(ctx context.Context, id string) (library.RefreshReport, error)
	MarkRead(ctx context.Context, id string, keys []string, read bool) (domain.Series, error)
	MarkReadUpTo(ctx context.Context, id string, upTo float64) (domain.Series, error)
	MarkSeen(ctx context.Context, id string) (domain.Series, error)
	ChangeSource(ctx context.Context, id, rawURL string, force bool) (domain.Series, error)
	RefreshCover(ctx context.Context, id string) (domain.Series, error)
	UploadCover(ctx context.Context, id string, data []byte) (domain.Series, error)
	Export() library.ExportFile
	Import(ctx context.Context, data []byte) (library.ImportReport, error)
}

// Jobs runs background "refresh all" jobs (implemented by
// *scheduler.Scheduler). Nil disables POST /refresh.
type Jobs interface {
	StartRefreshAll() (scheduler.Job, error)
	JobByID(id string) (scheduler.Job, error)
}

// Recipes manages the user recipes (implemented by *library.Recipes).
// Nil disables the /recipes routes.
type Recipes interface {
	List() ([]library.RecipeInfo, error)
	Get(name string) ([]byte, error)
	Save(ctx context.Context, name string, data []byte) error
	Delete(ctx context.Context, name string) error
	Test(ctx context.Context, data []byte, rawURL string) (library.RecipeTest, error)
	Generate(ctx context.Context, site, rawURL string) (library.GeneratedRecipe, error)
	CanGenerate() bool
}

// Covers reads local covers (implemented by *storage.Store).
type Covers interface {
	ReadCover(id string) ([]byte, string, error)
}

// Options configures the API.
type Options struct {
	AuthToken string       // when set, required as "Authorization: Bearer <token>"
	Version   string       // reported by /health
	Logger    *slog.Logger // default discards
	// LoopbackOnly rejects requests whose Host is not localhost/127.0.0.1/::1
	// (desktop app mode), which blocks DNS-rebinding attacks.
	LoopbackOnly bool
	// Mode is reported by /health: "server" (default), "app" (browser app
	// mode, set when OnQuit is) or "desktop" (the Wails app).
	Mode string
	// OnQuit, when set (desktop app mode), enables POST /app/quit, which
	// calls it to stop the server.
	OnQuit func()
	// OnShow, when set (the desktop app), enables POST /app/show, which
	// brings its window back: opening the app again shows the running one.
	OnShow func()
	// Jobs runs "refresh all" in the background (POST /refresh, GET /jobs).
	Jobs Jobs
	// Recipes enables the recipe routes (RF-53).
	Recipes Recipes
	// Settings enables GET/PUT /settings (SPEC §6.3).
	Settings Settings
	// Notifier enables POST /settings/notifications/test (RF-26).
	Notifier Notifier
	// Solvers names the anti-bot challenge solvers, reported by /health.
	Solvers []string
	// Enricher enables the AniList/MangaUpdates routes (RF-57).
	Enricher Enricher
	// SaveFile (desktop app) shows a native "save as" dialog for name,
	// writes data there and returns the path, or ErrSaveCanceled. It enables
	// POST /app/export.
	SaveFile func(ctx context.Context, name string, data []byte) (string, error)
}

// maxBodyBytes limits request bodies; requests are tiny JSON documents.
const maxBodyBytes = 1 << 20

type server struct {
	svc    Service
	covers Covers
	opts   Options
	log    *slog.Logger
}

// New returns the HTTP handler for the API.
func New(svc Service, covers Covers, opts Options) http.Handler {
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	s := &server{svc: svc, covers: covers, opts: opts, log: opts.Logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("POST /api/v1/extract/preview", s.preview)
	mux.HandleFunc("GET /api/v1/series", s.listSeries)
	mux.HandleFunc("GET /api/v1/sources", s.listSources)
	mux.HandleFunc("POST /api/v1/series", s.addSeries)
	mux.HandleFunc("GET /api/v1/series/{id}", s.getSeries)
	mux.HandleFunc("PATCH /api/v1/series/{id}", s.editSeries)
	mux.HandleFunc("DELETE /api/v1/series/{id}", s.removeSeries)
	mux.HandleFunc("POST /api/v1/series/{id}/refresh", s.refreshSeries)
	mux.HandleFunc("POST /api/v1/series/{id}/read", s.markRead)
	mux.HandleFunc("POST /api/v1/series/{id}/seen", s.markSeen)
	mux.HandleFunc("GET /api/v1/covers/{id}", s.cover)
	mux.HandleFunc("POST /api/v1/series/{id}/cover/refresh", s.refreshCover)
	mux.HandleFunc("POST /api/v1/series/{id}/source", s.changeSource)
	mux.HandleFunc("PUT /api/v1/series/{id}/cover", s.uploadCover)
	mux.HandleFunc("GET /api/v1/export", s.export)
	mux.HandleFunc("POST /api/v1/import", s.importLibrary)
	if opts.OnQuit != nil {
		mux.HandleFunc("POST /api/v1/app/quit", s.quit)
	}
	if opts.OnShow != nil {
		mux.HandleFunc("POST /api/v1/app/show", s.show)
	}
	if opts.SaveFile != nil {
		mux.HandleFunc("POST /api/v1/app/export", s.saveExport)
	}
	if opts.Recipes != nil {
		s.recipeRoutes(mux)
	}
	if s.opts.Settings != nil {
		s.settingsRoutes(mux)
	}
	if s.opts.Enricher != nil {
		s.enrichRoutes(mux)
	}
	if opts.Jobs != nil {
		mux.HandleFunc("POST /api/v1/refresh", s.refreshAll)
		mux.HandleFunc("GET /api/v1/jobs/{id}", s.job)
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeErrorCode(w, http.StatusNotFound, "not_found", "no such endpoint")
	})
	return s.logRequests(s.guardOrigin(s.authenticate(mux)))
}

// guardOrigin protects against cross-site requests from pages open in the
// user's browser: requests that change data must come from the same origin
// (browsers always send Origin on cross-site POST/PATCH/DELETE; curl sends
// none), and in loopback mode the Host must be a loopback name.
func (s *server) guardOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.opts.LoopbackOnly && !isLoopbackHost(r.Host) {
			writeErrorCode(w, http.StatusForbidden, "forbidden_host", "host not allowed")
			return
		}
		unsafe := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
		if origin := r.Header.Get("Origin"); unsafe && origin != "" && !sameHost(origin, r.Host) {
			writeErrorCode(w, http.StatusForbidden, "forbidden_origin", "cross-origin request refused")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sameHost(origin, host string) bool {
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, host)
}

func isLoopbackHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// authenticate enforces AuthToken on every route except /health. Covers
// also accept ?token=, because an <img> cannot send headers; request logs
// never include the query string.
func (s *server) authenticate(next http.Handler) http.Handler {
	if s.opts.AuthToken == "" {
		return next
	}
	want := []byte("Bearer " + s.opts.AuthToken)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		if got == "" && r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/covers/") {
			got = "Bearer " + r.URL.Query().Get("token")
		}
		if r.URL.Path != "/api/v1/health" && subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			writeErrorCode(w, http.StatusUnauthorized, "unauthorized", "missing or invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// logRequests writes one structured line per request (never bodies).
func (s *server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		level := slog.LevelInfo
		if rec.status >= 500 {
			level = slog.LevelError
		}
		s.log.Log(r.Context(), level, "http request", "method", r.Method,
			"path", strings.TrimPrefix(r.URL.Path, "/api/v1"), "status", rec.status,
			"duration_ms", time.Since(start).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Health reports the server status; Mode is "app" for the desktop app
// (the UI then offers a "quit" button) and "server" otherwise.
type Health struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Mode    string `json:"mode"`
	LLM     bool   `json:"llm"` // a language model can generate recipes
	// Solvers are the anti-bot challenge solvers in use (RF-56), e.g.
	// ["flaresolverr", "browser"]; empty when protected sites just fail.
	Solvers []string `json:"solvers"`
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	mode := s.opts.Mode
	switch {
	case mode != "":
	case s.opts.OnQuit != nil:
		mode = "app"
	default:
		mode = "server"
	}
	llm := s.opts.Recipes != nil && s.opts.Recipes.CanGenerate()
	solvers := s.opts.Solvers
	if solvers == nil {
		solvers = []string{}
	}
	writeJSON(w, http.StatusOK, Health{Status: "ok", Version: s.opts.Version, Mode: mode, LLM: llm, Solvers: solvers})
}

func (s *server) quit(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusAccepted)
	go s.opts.OnQuit() // after the response is written
}

func (s *server) show(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusAccepted)
	go s.opts.OnShow() // after the response is written
}

// refreshAll starts (or returns the running) refresh-all job (RF-21).
func (s *server) refreshAll(w http.ResponseWriter, r *http.Request) {
	job, err := s.opts.Jobs.StartRefreshAll()
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *server) job(w http.ResponseWriter, r *http.Request) {
	job, err := s.opts.Jobs.JobByID(r.PathValue("id"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}
