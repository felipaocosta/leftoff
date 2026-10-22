package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

const coversDir = "covers"

var (
	// ErrNotImage means cover data is not a supported image format.
	ErrNotImage = errors.New("cover is not a supported image")
	// coverIDRe restricts ids used in file names (ULIDs and test ids).
	coverIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// coverFormats maps sniffed content types to file extensions. The original
// bytes are stored as-is: the stdlib cannot encode WebP (SPEC §6.1).
var coverFormats = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// SaveCover stores the cover of series id, replacing any previous one, and
// returns its path relative to the data dir (e.g. "covers/<id>.jpg").
func (s *Store) SaveCover(ctx context.Context, id string, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("save cover %s: %w", id, err)
	}
	if !coverIDRe.MatchString(id) {
		return "", fmt.Errorf("save cover: invalid id %q", id)
	}
	ext, ok := coverFormats[http.DetectContentType(data)]
	if !ok {
		return "", fmt.Errorf("save cover %s: %w", id, ErrNotImage)
	}
	rel := filepath.Join(coversDir, id+ext)
	if err := writeFileAtomic(filepath.Join(s.dir, rel), data, filePerm); err != nil {
		return "", fmt.Errorf("save cover %s: %w", id, err)
	}
	if err := s.removeCovers(id, ext); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// ReadCover returns the stored cover of series id and its content type, or
// an error wrapping os.ErrNotExist when there is none.
func (s *Store) ReadCover(id string) ([]byte, string, error) {
	if !coverIDRe.MatchString(id) {
		return nil, "", fmt.Errorf("read cover %q: %w", id, os.ErrNotExist)
	}
	for contentType, ext := range coverFormats {
		data, err := os.ReadFile(filepath.Join(s.dir, coversDir, id+ext))
		if err == nil {
			return data, contentType, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, "", fmt.Errorf("read cover %s: %w", id, err)
		}
	}
	return nil, "", fmt.Errorf("read cover %s: %w", id, os.ErrNotExist)
}

// DeleteCover removes the stored cover of series id, if any.
func (s *Store) DeleteCover(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("delete cover %s: %w", id, err)
	}
	if !coverIDRe.MatchString(id) {
		return nil // such an id never had a cover file
	}
	return s.removeCovers(id, "")
}

// removeCovers deletes the cover files of id except the one with keepExt.
func (s *Store) removeCovers(id, keepExt string) error {
	for _, ext := range coverFormats {
		if ext == keepExt {
			continue
		}
		err := os.Remove(filepath.Join(s.dir, coversDir, id+ext))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove old cover %s%s: %w", id, ext, err)
		}
	}
	return nil
}
