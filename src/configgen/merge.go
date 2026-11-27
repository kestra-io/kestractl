package configgen

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// setLiteralBlockStyle walks a yaml.Node tree and sets any scalar value
// whose key matches targetKey to literal block style (|).
func setLiteralBlockStyle(node *yaml.Node, targetKey string) {
	if node == nil {
		return
	}
	if node.Kind == yaml.DocumentNode {
		for _, child := range node.Content {
			setLiteralBlockStyle(child, targetKey)
		}
		return
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			if keyNode.Value == targetKey && valNode.Kind == yaml.ScalarNode && strings.Contains(valNode.Value, "\n") {
				valNode.Style = yaml.LiteralStyle
			}
			setLiteralBlockStyle(valNode, targetKey)
		}
	}
	if node.Kind == yaml.SequenceNode {
		for _, child := range node.Content {
			setLiteralBlockStyle(child, targetKey)
		}
	}
}

// deepMerge recursively merges src into dst. src wins on conflicts.
// Both maps are not modified; a new map is returned.
func deepMerge(dst, src map[string]any) map[string]any {
	result := make(map[string]any, len(dst))
	for k, v := range dst {
		result[k] = v
	}
	for k, v := range src {
		if srcMap, ok := v.(map[string]any); ok {
			if dstMap, ok := result[k].(map[string]any); ok {
				result[k] = deepMerge(dstMap, srcMap)
				continue
			}
		}
		result[k] = v
	}
	return result
}

// replacePlaceholders walks a map recursively and replaces __KEY__ placeholders
// in string values using the provided replacements map.
func replacePlaceholders(m map[string]any, replacements map[string]string) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case map[string]any:
			result[k] = replacePlaceholders(val, replacements)
		case string:
			s := val
			for placeholder, replacement := range replacements {
				s = strings.ReplaceAll(s, placeholder, replacement)
			}
			result[k] = s
		case []any:
			result[k] = replaceInSlice(val, replacements)
		default:
			result[k] = v
		}
	}
	return result
}

func replaceInSlice(slice []any, replacements map[string]string) []any {
	result := make([]any, len(slice))
	for i, v := range slice {
		switch val := v.(type) {
		case map[string]any:
			result[i] = replacePlaceholders(val, replacements)
		case string:
			s := val
			for placeholder, replacement := range replacements {
				s = strings.ReplaceAll(s, placeholder, replacement)
			}
			result[i] = s
		default:
			result[i] = v
		}
	}
	return result
}
