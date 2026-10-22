// Package fetch downloads web pages politely (SPEC §5.8, RULES §6).
//
// Client applies, per request: http/https-only URLs, a redirect limit, a
// response size limit, a per-domain gate (one request at a time per host and
// a minimum interval between them), retries with backoff on transient errors
// and an in-memory response cache. It maps "not found" and anti-bot
// challenges to domain.ErrNotFound and domain.ErrBlocked.
//
// It does NOT parse HTML, solve Cloudflare challenges (FlareSolverr/WebView
// come in a later phase) or persist anything to disk.
package fetch
