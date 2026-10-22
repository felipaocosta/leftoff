package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrNewerSchema means the file was written by a newer version of the app.
// It is never rewritten, so no data written by that version is lost.
var ErrNewerSchema = errors.New("library schema is newer than this version supports")

// migration converts a raw library document from schema N to N+1 in place.
type migration func(doc map[string]json.RawMessage) error

// migrations returns the registered migrations; index i converts schema
// i+1 into i+2. Add new steps at the end and bump CurrentSchema.
func migrations() []migration {
	return []migration{migrateV1ToV2, migrateV2ToV3}
}

// migrateV1ToV2 adds series.external (RF-57). The field is new and
// optional: every v1 series reads as "never looked up" (null), so the
// step only checks that the document has a series list. Exports and
// imports of v1 files keep working because nothing is renamed.
func migrateV1ToV2(doc map[string]json.RawMessage) error {
	var series []json.RawMessage
	if raw, ok := doc["series"]; ok {
		if err := json.Unmarshal(raw, &series); err != nil {
			return fmt.Errorf("series: %w", err)
		}
	}
	return nil
}

// legacyCheckErrors gives a code to the check errors written before schema
// 3, by the text this app used to write (SPEC §6.2).
var legacyCheckErrors = []struct{ text, code string }{
	{"suspiciously few chapters", "source_changed"},
	{"anti-bot", "blocked"},
	{"site unreachable", "upstream_error"},
	{"required field not found", "extraction_incomplete"},
}

// migrateV2ToV3 adds check_error.code: each stored check error gets the
// code its message implies, or "". Everything else is kept as it is.
func migrateV2ToV3(doc map[string]json.RawMessage) error {
	var series []map[string]json.RawMessage
	if raw, ok := doc["series"]; ok {
		if err := json.Unmarshal(raw, &series); err != nil {
			return fmt.Errorf("series: %w", err)
		}
	}
	for i, s := range series {
		var ce map[string]json.RawMessage
		if err := json.Unmarshal(s["check_error"], &ce); err != nil || ce == nil {
			continue // null, absent or not an object: nothing to code
		}
		if _, has := ce["code"]; has {
			continue
		}
		var msg string
		_ = json.Unmarshal(ce["message"], &msg) // a non-string message just gets no code
		code := ""
		for _, l := range legacyCheckErrors {
			if strings.Contains(msg, l.text) {
				code = l.code
				break
			}
		}
		ce["code"], _ = json.Marshal(code) // a string always encodes
		if series[i]["check_error"], _ = json.Marshal(ce); series[i]["check_error"] == nil {
			return fmt.Errorf("series %d: encode check_error", i)
		}
	}
	if series == nil {
		return nil
	}
	raw, err := json.Marshal(series)
	if err != nil {
		return fmt.Errorf("series: %w", err)
	}
	doc["series"] = raw
	return nil
}

// migrate upgrades doc to target using steps and reports whether anything
// changed. It refuses documents newer than target.
func migrate(doc map[string]json.RawMessage, target int, steps []migration) (bool, error) {
	from, err := schemaOf(doc)
	if err != nil {
		return false, err
	}
	if from > target {
		return false, fmt.Errorf("schema %d > %d: %w", from, target, ErrNewerSchema)
	}
	if target-1 > len(steps) {
		return false, fmt.Errorf("no migration path to schema %d (have %d steps)", target, len(steps))
	}
	for v := from; v < target; v++ {
		if err := steps[v-1](doc); err != nil {
			return false, fmt.Errorf("migrate schema %d to %d: %w", v, v+1, err)
		}
		doc["schema"] = json.RawMessage(fmt.Sprint(v + 1))
	}
	return from < target, nil
}

func schemaOf(doc map[string]json.RawMessage) (int, error) {
	raw, ok := doc["schema"]
	if !ok {
		return 0, errors.New("library has no schema field")
	}
	var v int
	if err := json.Unmarshal(raw, &v); err != nil || v < 1 {
		return 0, fmt.Errorf("invalid schema value %s", raw)
	}
	return v, nil
}
