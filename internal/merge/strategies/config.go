package strategies

import (
	"encoding/json"
	"fmt"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	"github.com/provasign/fuse/internal/core"
)

// ConfigMergeResult is the output of a config (JSON/YAML/TOML) deep merge.
type ConfigMergeResult struct {
	Merged      string
	HasConflict bool
	Confidence  float64
}

// ConfigMerge performs a deep three-way merge over JSON, YAML, or TOML data
// preserving the source format.
//
//  1. Parse all three sides into generic Go values.
//  2. Recursively merge: for maps, walk keys; for scalars, apply
//     ThreeWayString; for slices, take ours if changed else theirs.
//  3. Re-serialize using the language's encoder. Comments are *not*
//     preserved (the cost would exceed value).
//
// On parse failure of any side, falls back to line-level merge of the raw
// text.
func ConfigMerge(lang core.LanguageKey, base, ours, theirs string) ConfigMergeResult {
	parse := parserFor(lang)
	encode := encoderFor(lang)
	if parse == nil || encode == nil {
		// Fallback to line merge
		r := LineMerge(base, ours, theirs)
		return ConfigMergeResult(r)
	}

	baseV, errB := parse(base)
	oursV, errO := parse(ours)
	theirsV, errT := parse(theirs)
	if errB != nil || errO != nil || errT != nil {
		r := LineMerge(base, ours, theirs)
		return ConfigMergeResult{Merged: r.Merged, HasConflict: r.HasConflict, Confidence: r.Confidence * 0.8}
	}

	merged, conflict := mergeValue(baseV, oursV, theirsV)
	out, err := encode(merged)
	if err != nil {
		r := LineMerge(base, ours, theirs)
		return ConfigMergeResult{Merged: r.Merged, HasConflict: r.HasConflict, Confidence: r.Confidence * 0.8}
	}
	conf := 0.85
	if !conflict {
		conf = 0.9
	} else {
		conf = 0.0
	}
	return ConfigMergeResult{Merged: out, HasConflict: conflict, Confidence: conf}
}

type parseFn func(string) (any, error)
type encodeFn func(any) (string, error)

func parserFor(lang core.LanguageKey) parseFn {
	switch lang {
	case core.LangJSON:
		return func(s string) (any, error) {
			if s == "" {
				return nil, nil
			}
			var v any
			err := json.Unmarshal([]byte(s), &v)
			return v, err
		}
	case core.LangYAML:
		return func(s string) (any, error) {
			if s == "" {
				return nil, nil
			}
			var v any
			err := yaml.Unmarshal([]byte(s), &v)
			return normalizeMap(v), err
		}
	case core.LangTOML:
		return func(s string) (any, error) {
			if s == "" {
				return nil, nil
			}
			var v any
			err := toml.Unmarshal([]byte(s), &v)
			return v, err
		}
	}
	return nil
}

func encoderFor(lang core.LanguageKey) encodeFn {
	switch lang {
	case core.LangJSON:
		return func(v any) (string, error) {
			out, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return "", err
			}
			return string(out) + "\n", nil
		}
	case core.LangYAML:
		return func(v any) (string, error) {
			out, err := yaml.Marshal(v)
			return string(out), err
		}
	case core.LangTOML:
		return func(v any) (string, error) {
			out, err := toml.Marshal(v)
			return string(out), err
		}
	}
	return nil
}

// normalizeMap converts map[interface{}]interface{} (yaml.v3 quirk) to
// map[string]any recursively.
func normalizeMap(v any) any {
	switch m := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[fmt.Sprintf("%v", k)] = normalizeMap(val)
		}
		return out
	case map[string]any:
		for k, val := range m {
			m[k] = normalizeMap(val)
		}
		return m
	case []any:
		for i, val := range m {
			m[i] = normalizeMap(val)
		}
		return m
	}
	return v
}

// mergeValue recursively merges three values. Returns (merged, hadConflict).
func mergeValue(base, ours, theirs any) (any, bool) {
	if isMap(base) && isMap(ours) && isMap(theirs) {
		return mergeMaps(asMap(base), asMap(ours), asMap(theirs))
	}
	if isMap(ours) && isMap(theirs) && base == nil {
		return mergeMaps(map[string]any{}, asMap(ours), asMap(theirs))
	}
	// scalar / slice / mismatched type: apply three-way string equality.
	if valEqual(ours, theirs) {
		return ours, false
	}
	if valEqual(base, ours) {
		return theirs, false
	}
	if valEqual(base, theirs) {
		return ours, false
	}
	// conflict — embed both sides as a structured marker.
	return map[string]any{
		"__fuse_conflict__": true,
		"ours":              ours,
		"theirs":            theirs,
	}, true
}

func mergeMaps(base, ours, theirs map[string]any) (map[string]any, bool) {
	keys := map[string]bool{}
	for k := range base {
		keys[k] = true
	}
	for k := range ours {
		keys[k] = true
	}
	for k := range theirs {
		keys[k] = true
	}
	out := make(map[string]any, len(keys))
	hadConflict := false
	for k := range keys {
		b, hasB := base[k]
		o, hasO := ours[k]
		t, hasT := theirs[k]
		switch {
		case !hasO && !hasT:
			// both deleted; drop
		case !hasO:
			// ours deleted, theirs present → accept deletion if theirs == base
			if hasB && valEqual(b, t) {
				continue
			}
			out[k] = t
		case !hasT:
			if hasB && valEqual(b, o) {
				continue
			}
			out[k] = o
		default:
			merged, conflict := mergeValue(b, o, t)
			out[k] = merged
			if conflict {
				hadConflict = true
			}
		}
	}
	return out, hadConflict
}

func isMap(v any) bool {
	_, ok := v.(map[string]any)
	if ok {
		return true
	}
	_, ok = v.(map[any]any)
	return ok
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	if m, ok := v.(map[any]any); ok {
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[fmt.Sprintf("%v", k)] = val
		}
		return out
	}
	return nil
}

func valEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}
