//go:build perf

package storage

import (
	"context"
	"os"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// BenchmarkUpdate measures one write of the library in PERF_DATA_DIR
// (written by internal/app TestPerf).
func BenchmarkUpdate(b *testing.B) {
	dir := os.Getenv("PERF_DATA_DIR")
	if dir == "" {
		b.Skip("PERF_DATA_DIR not set")
	}
	s, err := Open(context.Background(), dir, domain.SystemClock{}, Options{})
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if err := s.Update(context.Background(), func(*Library) error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}
