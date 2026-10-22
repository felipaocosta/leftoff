package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
)

// Extra holds JSON fields unknown to this version, so they survive a
// read-modify-write cycle (SPEC §6.4 item 5, ADR 0001).
type Extra map[string]json.RawMessage

// MarshalWithExtra encodes v (a struct without a custom MarshalJSON) and
// appends the extra fields after the known ones, in key order.
func MarshalWithExtra(v any, extra Extra) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal known fields: %w", err)
	}
	if len(extra) == 0 {
		return b, nil
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.Write(b[:len(b)-1]) // drop the closing '}'
	for i, k := range keys {
		if i > 0 || len(b) > 2 {
			buf.WriteByte(',')
		}
		name, _ := json.Marshal(k) // a string always encodes
		buf.Write(name)
		buf.WriteByte(':')
		buf.Write(extra[k])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// UnmarshalWithExtra decodes data into v (a pointer to a struct without a
// custom UnmarshalJSON) and returns the fields that v does not declare.
// The usual case, no unknown field, takes a single pass over data.
func UnmarshalWithExtra(data []byte, v any) (Extra, error) {
	if onlyKnownFields(data, v) {
		return nil, nil
	}
	if err := json.Unmarshal(data, v); err != nil {
		return nil, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, fmt.Errorf("decode fields: %w", err)
	}
	known := jsonFieldNames(reflect.TypeOf(v).Elem())
	for k := range all {
		// encoding/json matches keys case-insensitively, so do we.
		if known[strings.ToLower(k)] {
			delete(all, k)
		}
	}
	if len(all) == 0 {
		return nil, nil
	}
	return all, nil
}

// onlyKnownFields decodes data into v and reports whether that worked
// with no unknown field and nothing after the value. When it did not, the
// caller decodes again the slow way (which also reports real errors).
func onlyKnownFields(data []byte, v any) bool {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if dec.Decode(v) != nil {
		return false
	}
	_, err := dec.Token()
	return err == io.EOF
}

// jsonFieldNames returns the lowercase JSON names of the fields of struct t.
func jsonFieldNames(t reflect.Type) map[string]bool {
	names := make(map[string]bool, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" || !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		names[strings.ToLower(name)] = true
	}
	return names
}
