package mcp

import (
	"reflect"
	"testing"
)

func toolProperty(t *testing.T, definitions []ToolDefinition, toolName, property string) map[string]any {
	t.Helper()
	for _, definition := range definitions {
		if definition.Name != toolName {
			continue
		}
		properties, ok := definition.InputSchema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s properties have type %T", toolName, definition.InputSchema["properties"])
		}
		schema, ok := properties[property].(map[string]any)
		if !ok {
			t.Fatalf("%s.%s schema has type %T", toolName, property, properties[property])
		}
		return schema
	}
	t.Fatalf("tool %s not found", toolName)
	return nil
}

func TestToolDefinitionsUseConfiguredValues(t *testing.T) {
	definitions := getToolDefinitions([]string{"Planning", "Customer Review"}, []string{"Alice", "Bob"})

	interfaces := toolProperty(t, definitions, "dossier_update", "interfaces")
	items := interfaces["items"].(map[string]any)
	if got, want := items["enum"], []string{"Planning", "Customer Review"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("interface enum = %v, want %v", got, want)
	}

	lead := toolProperty(t, definitions, "dossier_update", "lead")
	if got, want := lead["enum"], []string{"", "Alice", "Bob"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lead enum = %v, want %v", got, want)
	}
}

func TestToolDefinitionsHandleEmptyConfiguredLists(t *testing.T) {
	definitions := getToolDefinitions([]string{}, []string{})
	interfaces := toolProperty(t, definitions, "dossier_update", "interfaces")
	if interfaces["maxItems"] != 0 {
		t.Fatalf("empty interfaces schema = %v, want maxItems 0", interfaces)
	}
	if _, exists := toolProperty(t, definitions, "dossier_update", "lead")["enum"]; exists {
		t.Fatal("empty lead configuration should preserve free-form values")
	}
}
