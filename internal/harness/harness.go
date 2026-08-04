package harness

import (
	"dossier/internal/core"
	"fmt"
	"strings"
)

// Registry manages the set of supported client harnesses.
type Registry struct {
	harnesses []core.Harness
}

// NewRegistry instantiates the harness registry.
func NewRegistry(dossierHome string) *Registry {
	return &Registry{
		harnesses: []core.Harness{
			NewClaudeCodeHarness(dossierHome),
			NewPiHarness(dossierHome),
		},
	}
}

// All returns all registered harnesses.
func (r *Registry) All() []core.Harness {
	return r.harnesses
}

// Get retrieves a harness by name.
func (r *Registry) Get(name string) (core.Harness, error) {
	for _, h := range r.harnesses {
		if h.Name() == name {
			return h, nil
		}
	}
	return nil, fmt.Errorf("harness %q not found", name)
}

// isHookConfigured checks if a hook has a command entry that matches cmd exactly.
func isHookConfigured(existingVal any, cmd string) bool {
	arr, ok := existingVal.([]any)
	if !ok {
		return false
	}
	for _, item := range arr {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		hooksVal, ok := itemMap["hooks"]
		if !ok {
			continue
		}
		hooksArr, ok := hooksVal.([]any)
		if !ok {
			continue
		}
		for _, h := range hooksArr {
			hMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			hCmd, _ := hMap["command"].(string)
			hType, _ := hMap["type"].(string)
			if hType == "command" && hCmd == cmd {
				return true
			}
		}
	}
	return false
}

// updateHookArray parses, updates (preserving existing items), and returns the new hook array.
// Claude Code hook arrays are keyed by a "matcher" object; new hooks attach to the "*" matcher.
func updateHookArray(existingVal any, cmd string, suffix string) []any {
	var arr []any
	if existingArr, ok := existingVal.([]any); ok {
		arr = existingArr
	}

	// First, check if a hook containing the command suffix is already present.
	// If it is, update its path to the new command.
	updated := false
	for _, item := range arr {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		hooksVal, ok := itemMap["hooks"]
		if !ok {
			continue
		}
		hooksArr, ok := hooksVal.([]any)
		if !ok {
			continue
		}
		for i, h := range hooksArr {
			hMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			hCmd, _ := hMap["command"].(string)
			hType, _ := hMap["type"].(string)
			if hType == "command" && strings.Contains(hCmd, suffix) {
				hMap["command"] = cmd
				hooksArr[i] = hMap
				updated = true
			}
		}
		if updated {
			itemMap["hooks"] = hooksArr
		}
	}

	if updated {
		return arr
	}

	// Not found, insert new command hook.
	newHook := map[string]any{
		"type":    "command",
		"command": cmd,
	}

	var targetMap map[string]any
	for _, item := range arr {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if m, _ := itemMap["matcher"].(string); m == "*" {
			targetMap = itemMap
			break
		}
	}

	if targetMap != nil {
		var hooksArr []any
		if hVal, ok := targetMap["hooks"]; ok {
			if hArr, ok := hVal.([]any); ok {
				hooksArr = hArr
			}
		}
		hooksArr = append(hooksArr, newHook)
		targetMap["hooks"] = hooksArr
	} else {
		arr = append(arr, map[string]any{
			"matcher": "*",
			"hooks":   []any{newHook},
		})
	}

	return arr
}
