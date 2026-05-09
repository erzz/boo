package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"

	"github.com/erzz/boo/internal/state"
)

// SetUITheme writes ui.theme=name into the YAML file at path,
// preserving comments, formatting, and the position of other keys.
// Creates the file if it doesn't exist.
//
// If the file is absent or empty, a minimal file containing only
// ui.theme is written. If the file already exists, the YAML node
// tree is parsed, the ui.theme scalar is found-or-created, and the
// document is re-encoded with 2-space indent — all other keys,
// comments, and ordering are preserved.
func SetUITheme(path, name string) error {
	if path == "" {
		return errors.New("config path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var out []byte
	if len(bytes.TrimSpace(data)) == 0 {
		// No existing content to preserve — write a minimal file.
		out = []byte("ui:\n  theme: " + name + "\n")
	} else {
		out, err = patchThemeNode(data, name)
		if err != nil {
			return fmt.Errorf("patch %s: %w", path, err)
		}
	}

	if err := state.WriteAtomic(path, out); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// patchThemeNode parses data as a YAML node tree, sets ui.theme=name,
// and re-encodes the document preserving all comments and key ordering.
func patchThemeNode(data []byte, name string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	// yaml.Unmarshal into a yaml.Node produces a DocumentNode whose
	// sole child is the top-level MappingNode.
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, errors.New("unexpected yaml structure: expected document node")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, errors.New("unexpected yaml structure: top-level is not a mapping")
	}

	// Find or create the "ui" mapping value.
	uiVal := findOrCreateMappingKey(root, "ui")

	// Within "ui", find or create the "theme" scalar and set its value.
	setMappingScalar(uiVal, "theme", name)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, fmt.Errorf("encode yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close encoder: %w", err)
	}
	return buf.Bytes(), nil
}

// findOrCreateMappingKey looks for key in a MappingNode's children
// (alternating key/value pairs) and returns its value node. If key is
// absent, a new key node and an empty MappingNode value are appended and
// the value node is returned.
func findOrCreateMappingKey(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	kn := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	vn := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	m.Content = append(m.Content, kn, vn)
	return vn
}

// setMappingScalar finds key inside a MappingNode and updates its scalar
// value. If key is absent a new key/value pair is appended.
func setMappingScalar(m *yaml.Node, key, value string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1].Value = value
			m.Content[i+1].Tag = "!!str"
			m.Content[i+1].Kind = yaml.ScalarNode
			return
		}
	}
	kn := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	vn := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	m.Content = append(m.Content, kn, vn)
}
