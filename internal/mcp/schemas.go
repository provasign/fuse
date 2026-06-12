package mcp

// ToolSchemas returns the four fuse tools. Descriptions are deliberately
// terse: these ride in every agent's context window for the whole session.
func ToolSchemas() []map[string]any {
	return []map[string]any{
		{
			"name":        "fuse_merge_check",
			"description": "Dry-run semantic merge of your committed HEAD against a ref. Answers 'will my work merge cleanly?' before you push: verdict, per-file conflicts, breaking changes. Nothing is written.",
			"inputSchema": objectSchema(nil, map[string]any{
				"ref": prop("string", "Ref to merge against (default origin/main). Can be another agent's branch."),
			}),
		},
		{
			"name":        "fuse_preview",
			"description": "Three-way semantic merge of in-memory content; returns the merged text and whether conflicts remain. Nothing is written.",
			"inputSchema": objectSchema([]string{"ours", "theirs"}, map[string]any{
				"base":   prop("string", "Common-ancestor content (empty for two-way)."),
				"ours":   prop("string", "Our side's content."),
				"theirs": prop("string", "Their side's content."),
				"path":   prop("string", "File path used for language detection (e.g. 'auth.go')."),
			}),
		},
		{
			"name":        "fuse_resolve",
			"description": "Work a conflict handoff prompt. Without 'resolution': returns the prompt so you can resolve in-context. With 'resolution': validates it (no markers, parses); 'apply' writes it to the conflicted file.",
			"inputSchema": objectSchema([]string{"prompt"}, map[string]any{
				"prompt":     prop("string", "Path to the .git/fuse handoff prompt file."),
				"resolution": prop("string", "Complete resolved file content."),
				"apply":      prop("boolean", "Write the validated resolution to the conflicted file."),
			}),
		},
		{
			"name":        "fuse_impact",
			"description": "Blast radius via the Grove graph: which symbols depend on a file or symbol you changed. Self-check before finishing a task.",
			"inputSchema": objectSchema([]string{"query"}, map[string]any{
				"query":    prop("string", "Repo-relative file path or symbol name."),
				"maxDepth": prop("integer", "Traversal depth (default 3)."),
			}),
		},
	}
}

func prop(typ, description string) map[string]any {
	return map[string]any{"type": typ, "description": description}
}

func objectSchema(required []string, props map[string]any) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
