// Package domain holds the core types of LeftOff (Series, Chapter,
// Status, extraction results) and pure functions over them: status
// normalization, stable chapter keys and derived fields.
//
// It does NOT perform I/O of any kind (no network, no disk) and imports no
// other package from this project. Persistence lives in storage, business
// rules that orchestrate several steps (add, refresh, merge) live in library.
package domain
