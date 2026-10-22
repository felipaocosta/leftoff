package app

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/felipaocosta/leftoff/internal/api"
	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/enrich"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/extract/adapters/liliana"
	"github.com/felipaocosta/leftoff/internal/extract/adapters/madara"
	"github.com/felipaocosta/leftoff/internal/extract/adapters/mangadex"
	"github.com/felipaocosta/leftoff/internal/extract/adapters/mangastream"
	"github.com/felipaocosta/leftoff/internal/extract/adapters/mgeko"
	"github.com/felipaocosta/leftoff/internal/extract/adapters/novelupdates"
	"github.com/felipaocosta/leftoff/internal/extract/heuristic"
	"github.com/felipaocosta/leftoff/internal/extract/llm"
	"github.com/felipaocosta/leftoff/internal/extract/recipe"
	"github.com/felipaocosta/leftoff/internal/fetch"
	"github.com/felipaocosta/leftoff/internal/fetch/browser"
	"github.com/felipaocosta/leftoff/internal/library"
	"github.com/felipaocosta/leftoff/internal/scheduler"
	"github.com/felipaocosta/leftoff/internal/storage"
	"github.com/felipaocosta/leftoff/web"
)

// newSolvers lists the anti-bot challenge solvers (RF-56): FlareSolverr
// first (automatic, no window), then the local browser, which opens a
// window only in interactive mode. The local browser, when enabled, is also
// returned as the renderer for JavaScript-only pages.
func newSolvers(cfg Config, logger *slog.Logger) ([]fetch.Solver, extract.Renderer) {
	var solvers []fetch.Solver
	if cfg.FlareSolverrURL != "" {
		solvers = append(solvers, fetch.NewFlareSolverr(cfg.FlareSolverrURL))
	}
	if cfg.BrowserSolver != "headless" && cfg.BrowserSolver != "interactive" {
		return solvers, nil
	}
	if !browser.Available() {
		logger.Warn("BROWSER_SOLVER is set but this build has no browser support (build tag rod)")
		return solvers, nil
	}
	b := browser.New(browser.Options{
		Bin:         cfg.BrowserBin,
		ProfileDir:  filepath.Join(cfg.DataDir, "browser-profile"),
		Interactive: cfg.BrowserSolver == "interactive",
	})
	return append(solvers, b), b
}

// newExtractor builds the fetch client and the extraction pipeline. The
// adapter list is assembled here explicitly (RULES §4).
func newExtractor(cfg Config, logger *slog.Logger, recipes extract.Recipes, transport http.RoundTripper, tab browser.Driver) (*fetch.Client, *extract.Extractor, extract.Renderer) {
	solvers, renderer := newSolvers(cfg, logger)
	if tab != nil { // the Android WebView: renders pages and passes challenges (RF-56)
		d := browser.NewDriven(tab, browser.DrivenOptions{Interactive: true})
		solvers = append(solvers, d)
		if renderer == nil {
			renderer = d
		}
	}
	client := fetch.New(fetch.Options{
		UserAgent:   cfg.UserAgent,
		MinInterval: cfg.MinInterval,
		Logger:      logger,
		Solvers:     solvers,
		Transport:   transport,
	})
	ex := extract.New(client, extract.Options{
		Adapters: adapters(cfg, domain.SystemClock{}),
		Fallback: heuristic.New(),
		Renderer: renderer,
		Recipes:  recipes,
		Logger:   logger,
	})
	return client, ex, renderer
}

// adapters is the adapter list, in the order the pipeline tries them. The
// clock turns relative release dates into dates.
func adapters(cfg Config, clock domain.Clock) []extract.Adapter {
	return []extract.Adapter{
		mangadex.New(cfg.MangaDexLangs...), novelupdates.New(), mgeko.New(), liliana.New(), mangastream.New(),
		madara.New(clock),
	}
}

// ExtractCommand prints the extraction result of rawURL (diagnostics CLI).
func ExtractCommand(ctx context.Context, cfg Config, logger *slog.Logger, rawURL string, out io.Writer) error {
	store, err := storage.Open(ctx, cfg.DataDir, domain.SystemClock{}, storage.Options{})
	if err != nil {
		return err
	}
	_, ex, _ := newExtractor(cfg, logger, loadRecipes(store, logger), nil, nil)
	res, err := ex.Extract(ctx, rawURL)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

// newApp opens the data dir and assembles storage, extraction, library,
// scheduler and API into the root HTTP handler. quit stops the server
// (app mode "quit").
func newApp(ctx context.Context, cfg Config, logger *slog.Logger, so ServeOptions, quit func()) (http.Handler, *scheduler.Scheduler, error) {
	clock := domain.SystemClock{}
	store, err := storage.Open(ctx, cfg.DataDir, clock, storage.Options{})
	if err != nil {
		return nil, nil, err
	}
	book := loadRecipes(store, logger)
	client, ex, renderer := newExtractor(cfg, logger, book, so.Transport, so.Browser)
	svc := library.New(store, ex, clock, library.Options{Fetcher: client, Logger: logger})
	settings := library.NewSettings(store, cfg.Notify).WithEnrichEnv(cfg.EnrichEnabled)
	enricher := library.NewEnricher(svc, settings, []enrich.Finder{
		enrich.NewAniList(client, ""), enrich.NewMangaUpdates(client, ""),
	}, logger)
	svc.UseEnricher(enricher)
	announcer := library.NewAnnouncer(svc, settings, &http.Client{Timeout: notifyTimeout, Transport: so.Transport}, logger)
	sched := scheduler.New(svc, scheduler.Options{
		Interval: cfg.CheckInterval, CheckCompleted: cfg.CheckCompleted, Clock: clock, Logger: logger,
		Announcer: announcer,
	})
	apiOpts := api.Options{
		AuthToken: cfg.AuthToken, Version: so.Version, Logger: logger, Jobs: sched,
		Recipes:  newRecipes(cfg, logger, recipeDeps{store: store, book: book, fetcher: client, renderer: renderer, transport: so.Transport}).WithSeries(svc),
		Settings: settings, Notifier: announcer, Solvers: client.SolverNames(), Enricher: enricher,
	}
	if so.AppMode || so.Desktop {
		apiOpts.LoopbackOnly, apiOpts.OnQuit = true, quit
	}
	if so.Desktop {
		apiOpts.Mode, apiOpts.SaveFile = cmp.Or(so.Mode, "desktop"), so.SaveFile
		apiOpts.OnShow = so.OnShow
		settings.WithDesktop()
		if so.Mode == "android" {
			apiOpts.OnQuit = nil // Android closes apps itself
		}
	}
	return newRootHandler(api.New(svc, store, apiOpts), web.FS(), web.Built()), sched, nil
}

// recipeDeps are what the recipes service shares with the extractor.
type recipeDeps struct {
	store     *storage.Store
	book      *recipe.Book
	fetcher   fetch.Fetcher
	renderer  extract.Renderer // nil without a local browser
	transport http.RoundTripper
}

// newRecipes builds the recipes service, with recipe generation when a
// language model is configured (RF-54).
func newRecipes(cfg Config, logger *slog.Logger, d recipeDeps) *library.Recipes {
	clock := domain.SystemClock{}
	r := library.NewRecipes(d.store, d.book, d.fetcher, clock)
	if d.renderer != nil {
		r.WithRenderer(d.renderer)
	}
	if cfg.OllamaURL == "" {
		return r
	}
	c := llm.NewClient(cfg.OllamaURL, cfg.OllamaModel, &http.Client{Timeout: llmTimeout, Transport: d.transport})
	logger.Info("recipe generation enabled", "url", cfg.OllamaURL, "model", cfg.OllamaModel)
	return r.WithGenerator(llm.NewGenerator(c, clock, 0))
}

// notifyTimeout bounds one request to ntfy, Discord or Telegram.
const notifyTimeout = 15 * time.Second

// llmTimeout bounds one answer of the language model (a CPU may be slow).
const llmTimeout = 6 * time.Minute

// ServeOptions changes how serve listens; the zero value is server mode.
type ServeOptions struct {
	Version string // reported by /health and the logs
	Host    string // "" listens on all interfaces; app mode uses 127.0.0.1
	AppMode bool   // loopback-only API with POST /app/quit
	Desktop bool   // the Wails or Android app: like AppMode, plus the desktop settings
	Mode    string // /health mode with Desktop: "desktop" (default) or "android"
	// SaveFile (desktop) is the native "save as" dialog for the export.
	SaveFile func(ctx context.Context, name string, data []byte) (string, error)
	OnReady  func(addr string) // called once the listener is open
	// OnScheduler receives the scheduler before serving (the Android core
	// runs background checks with it).
	OnScheduler func(*scheduler.Scheduler)
	// AnyPortIfBusy falls back to a free port when cfg.Port is taken. The
	// desktop and Android apps use a fixed port so the page's origin, and
	// with it the preferences kept in the browser (theme, language,
	// filters), is the same at every launch.
	AnyPortIfBusy bool
	// OnShow brings the desktop window back (POST /app/show).
	OnShow func()
	// Browser is a tab the host app drives (the Android WebView): it renders
	// JavaScript pages and passes anti-bot checks, showing itself when one
	// needs the user. Nil on the other builds, which use the local browser.
	Browser browser.Driver
	// Transport, when set, carries every outgoing request (sites, AniList,
	// MangaUpdates, notifications, the language model). Only the end-to-end
	// tests set it, to answer with saved pages; nil is the network.
	Transport http.RoundTripper
}

// owner names this process in the data folder lock, for the message a
// second LeftOff shows ("desktop 6e9d011").
func (so ServeOptions) owner() string {
	mode := "server"
	switch {
	case so.Desktop:
		mode = cmp.Or(so.Mode, "desktop")
	case so.AppMode:
		mode = "app"
	}
	return mode + " " + cmp.Or(so.Version, "dev")
}

// Serve runs the HTTP server until ctx is canceled (or, in app mode, the
// UI asks to quit), then shuts down gracefully.
func Serve(ctx context.Context, cfg Config, logger *slog.Logger, so ServeOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// One process per data folder (RULES §7): a second app on the same
	// folder would overwrite this one's changes.
	lock, err := storage.LockDir(cfg.DataDir, so.owner())
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }() // the process may be exiting; the OS frees it anyway
	handler, sched, err := newApp(ctx, cfg, logger, so, cancel)
	if err != nil {
		return err
	}
	if so.OnScheduler != nil {
		so.OnScheduler(sched)
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// Refresh waits for the per-site rate limit and retries.
		WriteTimeout: 3 * time.Minute,
		IdleTimeout:  2 * time.Minute,
	}
	ln, err := listen(ctx, so.Host, cfg.Port, so.AnyPortIfBusy, logger)
	if err != nil {
		return err
	}
	lock.SetURL("http://" + ln.Addr().String() + "/") // so another app can ask this one to quit
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	schedDone := make(chan struct{})
	go func() { // owned by ctx: stops with the server
		defer close(schedDone)
		if cfg.CheckInterval > 0 {
			_ = sched.Run(ctx) // returns ctx.Err() on shutdown
			return
		}
		_ = sched.Idle(ctx) // checks disabled; jobs still work
	}()
	defer func() { cancel(); <-schedDone }()
	addr := ln.Addr().String()
	logger.Info("server started", "addr", addr, "data_dir", cfg.DataDir, "version", so.Version,
		"auth", cfg.AuthToken != "", "ui", web.Built(), "app_mode", so.AppMode)
	if so.OnReady != nil {
		so.OnReady(addr)
	}

	select {
	case err := <-errc:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}
	logger.Info("shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// listen opens host:port, or a free port when that one is taken and
// anyIfBusy is set.
func listen(ctx context.Context, host string, port int, anyIfBusy bool, logger *slog.Logger) (net.Listener, error) {
	lc := new(net.ListenConfig)
	ln, err := lc.Listen(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil && anyIfBusy && port != 0 {
		logger.Warn("port taken, using a free one: preferences kept in the page will not carry over", "port", port, "err", err)
		ln, err = lc.Listen(ctx, "tcp", net.JoinHostPort(host, "0"))
	}
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	return ln, nil
}

// loadRecipes reads the user recipes from DATA_DIR/recipes (RF-53).
// Invalid files are logged and skipped.
func loadRecipes(store *storage.Store, logger *slog.Logger) *recipe.Book {
	book := recipe.NewBook(domain.SystemClock{})
	files, err := store.Recipes()
	if err != nil {
		logger.Warn("load recipes", "err", err)
		return book
	}
	for _, err := range book.Load(files) {
		logger.Warn("invalid recipe skipped", "err", err)
	}
	return book
}
