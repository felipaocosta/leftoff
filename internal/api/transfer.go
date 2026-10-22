package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// maxImportBytes limits import files; a large library with thousands of
// chapters is a few megabytes.
const maxImportBytes = 64 << 20

// ErrSaveCanceled is returned by Options.SaveFile when the user closes
// the "save as" dialog.
var ErrSaveCanceled = errors.New("save canceled")

// export downloads the whole library as a JSON file (RF-43).
func (s *server) export(w http.ResponseWriter, r *http.Request) {
	name, data, err := s.exportFile()
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data) // the client may be gone; nothing left to do
}

// saveExport (desktop app) asks where to save the export with a native
// dialog and writes it there: the desktop WebView cannot download files.
func (s *server) saveExport(w http.ResponseWriter, r *http.Request) {
	name, data, err := s.exportFile()
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	path, err := s.opts.SaveFile(r.Context(), name, data)
	switch {
	case errors.Is(err, ErrSaveCanceled):
		writeJSON(w, http.StatusOK, map[string]any{"canceled": true})
	case err != nil:
		s.writeError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"canceled": false, "path": path})
	}
}

// exportFile is the export's file name and indented JSON.
func (s *server) exportFile() (string, []byte, error) {
	file := s.svc.Export()
	name := fmt.Sprintf("leftoff-%s.json", file.ExportedAt.UTC().Format("20060102-150405"))
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return "", nil, fmt.Errorf("encode export: %w", err)
	}
	return name, append(data, '\n'), nil
}

// importLibrary merges an uploaded export (raw JSON body) into the library.
func (s *server) importLibrary(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImportBytes))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	report, err := s.svc.Import(r.Context(), data)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
