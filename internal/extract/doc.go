// Package extract turns a URL into series metadata and a chapter list
// (SPEC §5): it normalizes the URL, picks a native Adapter (by URL, then by
// detecting the engine in the HTML), falls back to a generic extractor, and
// normalizes the result (absolute URLs, status, duplicate chapters) and its
// confidence.
//
// It does NOT know any specific site (adapters live in extract/adapters/*),
// persist anything or decide what goes into the library.
package extract
