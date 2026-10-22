package api

import (
	"context"
	"net/http"

	"github.com/felipaocosta/leftoff/internal/library"
)

// Settings reads and changes the user settings (implemented by
// *library.Settings). Nil disables the /settings routes.
type Settings interface {
	View() library.SettingsView
	Update(ctx context.Context, u library.SettingsUpdate) (library.SettingsView, error)
}

// Notifier sends a test notification to every configured channel
// (implemented by *library.Announcer). Nil disables the test route.
type Notifier interface {
	Test(ctx context.Context) ([]library.ChannelResult, error)
}

func (s *server) settingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/settings", s.getSettings)
	mux.HandleFunc("PUT /api/v1/settings", s.putSettings)
	if s.opts.Notifier != nil {
		mux.HandleFunc("POST /api/v1/settings/notifications/test", s.testNotifications)
	}
}

// testNotifications sends a test message with the saved settings and
// reports each channel; 400 when none is configured.
func (s *server) testNotifications(w http.ResponseWriter, r *http.Request) {
	results, err := s.opts.Notifier.Test(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *server) getSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.opts.Settings.View())
}

// putSettings saves the settings; masked secrets keep their stored values.
func (s *server) putSettings(w http.ResponseWriter, r *http.Request) {
	var u library.SettingsUpdate
	if !s.decode(w, r, &u) {
		return
	}
	saved, err := s.opts.Settings.Update(r.Context(), u)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}
