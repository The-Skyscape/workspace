package tools

// DefaultSchema returns a default empty schema for tools that don't require parameters
func DefaultSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

// SimpleSchema creates a simple schema with the given parameters
func SimpleSchema(params map[string]any) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": params,
	}

	// Extract required fields
	var required []string
	for name, param := range params {
		if p, ok := param.(map[string]any); ok {
			if r, exists := p["required"]; exists && r == true {
				required = append(required, name)
				delete(p, "required") // Remove from individual param as it goes in top-level
			}
		}
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}
