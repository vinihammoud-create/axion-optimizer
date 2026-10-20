package startup

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// seenVersion is the layout of the file that remembers which startup entries
// the user already had the chance to see.
const seenVersion = 1

type seenFile struct {
	Version int      `json:"version"`
	SavedAt string   `json:"savedAt"`
	IDs     []string `json:"ids"`
}

// Key is the form of an entry ID used to compare visits. Registry value
// names and task paths are case-insensitive in Windows.
func Key(id string) string { return strings.ToLower(strings.TrimSpace(id)) }

// ParseSeen reads the saved list of IDs. ok is false when there is no usable
// baseline: no file, an empty file, damaged JSON or an unknown layout. The
// caller then treats the visit as the first one and marks nothing as new.
func ParseSeen(b []byte) (ids map[string]bool, ok bool) {
	var f seenFile
	if err := json.Unmarshal(b, &f); err != nil || f.Version != seenVersion {
		return nil, false
	}
	ids = make(map[string]bool, len(f.IDs))
	for _, id := range f.IDs {
		if k := Key(id); k != "" {
			ids[k] = true
		}
	}
	return ids, true
}

// EncodeSeen builds the file content for the entries of this visit.
func EncodeSeen(keys map[string]bool, now time.Time) ([]byte, error) {
	ids := make([]string, 0, len(keys))
	for k := range keys {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	return json.MarshalIndent(seenFile{Version: seenVersion, SavedAt: now.UTC().Format(time.RFC3339), IDs: ids}, "", " ")
}

// Keys is the set of comparison keys of a list.
func Keys(items []Item) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[Key(it.ID)] = true
	}
	return m
}

// NewSince returns the keys of the entries that are not in seen. Without a
// baseline (the first visit) nothing is new.
func NewSince(seen map[string]bool, baselined bool, items []Item) map[string]bool {
	out := map[string]bool{}
	if !baselined {
		return out
	}
	for _, it := range items {
		if k := Key(it.ID); !seen[k] {
			out[k] = true
		}
	}
	return out
}
