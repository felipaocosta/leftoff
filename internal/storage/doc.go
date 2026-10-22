// Package storage persists the library as JSON in DATA_DIR (SPEC §6).
//
// It owns every data file: writes are atomic (tmp + fsync + rename) and
// serialized by a mutex, reads come from an in-memory snapshot, schema
// migrations run on open, and rotating backups are taken before migrations
// and at most once per backup interval while the app is in use.
//
// It does NOT contain business rules (adding series, merging chapters):
// callers express changes as functions passed to Store.Update.
package storage
