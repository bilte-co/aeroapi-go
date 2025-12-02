// Package specfmt provides utilities for refactoring OpenAPI 3.0 YAML specs
// to extract inline schemas into named components/schemas.
package specfmt

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// schemaRegistry tracks schemas for deduplication and naming.
type schemaRegistry struct {
	byFingerprint map[string]string
	existingNames map[string]struct{}
	schemasNode   *yaml.Node
}

func newSchemaRegistry(schemas *yaml.Node) *schemaRegistry {
	r := &schemaRegistry{
		byFingerprint: make(map[string]string),
		existingNames: make(map[string]struct{}),
		schemasNode:   schemas,
	}
	if schemas.Kind == yaml.MappingNode {
		for i := 0; i < len(schemas.Content); i += 2 {
			nameNode := schemas.Content[i]
			schemaNode := schemas.Content[i+1]
			name := nameNode.Value
			r.existingNames[name] = struct{}{}
			fp := schemaFingerprint(schemaNode)
			if fp != "" {
				r.byFingerprint[fp] = name
			}
		}
	}
	return r
}

func schemaFingerprint(n *yaml.Node) string {
	var b strings.Builder
	writeNodeFingerprint(&b, n)
	return b.String()
}

func writeNodeFingerprint(b *strings.Builder, n *yaml.Node) {
	if n == nil {
		b.WriteString("nil;")
		return
	}
	fmt.Fprintf(b, "K:%d;T:%s;V:%q;", n.Kind, n.Tag, n.Value)
	if len(n.Content) > 0 {
		b.WriteString("[")
		for _, c := range n.Content {
			writeNodeFingerprint(b, c)
			b.WriteString("|")
		}
		b.WriteString("]")
	}
}

func (r *schemaRegistry) componentizeSchema(schemaNode *yaml.Node, nameHint string, opts Options) (string, bool, error) {
	if schemaNode == nil || schemaNode.Kind != yaml.MappingNode {
		return "", false, nil
	}
	if isRefOnlySchema(schemaNode) {
		return "", false, nil
	}

	fp := schemaFingerprint(schemaNode)

	if existingName, ok := r.byFingerprint[fp]; ok {
		makeRefOnlySchema(schemaNode, existingName)
		if opts.Verbose {
			fmt.Printf("  Reusing existing schema %s (fingerprint match)\n", existingName)
		}
		return existingName, true, nil
	}

	candidate := nameHint
	if candidate == "" {
		candidate = "InlineSchema"
	}
	name := r.ensureUniqueName(candidate, fp)

	cloned := cloneNode(schemaNode)
	appendMapEntry(r.schemasNode, name, cloned)
	r.existingNames[name] = struct{}{}
	r.byFingerprint[fp] = name

	makeRefOnlySchema(schemaNode, name)

	if opts.Verbose {
		fmt.Printf("  Created components/schemas/%s\n", name)
	}
	return name, true, nil
}

func (r *schemaRegistry) ensureUniqueName(base, fp string) string {
	if existing := getMapValue(r.schemasNode, base); existing != nil {
		if schemaFingerprint(existing) == fp {
			return base
		}
	}

	name := base
	i := 2
	for {
		if _, exists := r.existingNames[name]; !exists {
			return name
		}
		name = fmt.Sprintf("%s%d", base, i)
		i++
	}
}

// Options configures the formatting behavior.
type Options struct {
	DryRun  bool
	Verbose bool
}

// FormatFile reads an OpenAPI YAML file, refactors inline response schemas
// into components/schemas, and writes the result to outPath.
func FormatFile(inPath, outPath string, opts Options) error {
	f, err := os.Open(inPath)
	if err != nil {
		return fmt.Errorf("open input: %w", err)
	}
	defer f.Close()

	root, err := parseYAML(f)
	if err != nil {
		return err
	}

	changed, err := RefactorInlineResponseSchemas(root, opts)
	if err != nil {
		return err
	}

	if opts.DryRun {
		if opts.Verbose {
			fmt.Printf("Dry-run: changes detected = %v\n", changed)
		}
		return nil
	}

	if !changed {
		if opts.Verbose {
			fmt.Println("No changes needed")
		}
		return nil
	}

	out, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer out.Close()

	if err := writeYAML(out, root); err != nil {
		return err
	}

	if opts.Verbose {
		fmt.Printf("Wrote refactored spec to %s\n", outPath)
	}
	return nil
}

func parseYAML(r io.Reader) (*yaml.Node, error) {
	var root yaml.Node
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("decode YAML: %w", err)
	}
	return &root, nil
}

func writeYAML(w io.Writer, root *yaml.Node) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	defer enc.Close()

	if err := enc.Encode(root); err != nil {
		return fmt.Errorf("encode YAML: %w", err)
	}
	return nil
}

// RefactorInlineResponseSchemas walks all paths/operations/responses and
// extracts inline schemas into components/schemas.
func RefactorInlineResponseSchemas(root *yaml.Node, opts Options) (bool, error) {
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return false, fmt.Errorf("expected YAML document")
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return false, fmt.Errorf("expected top-level mapping")
	}

	componentsNode := ensureMapValue(top, "components")
	schemasNode := ensureMapValue(componentsNode, "schemas")

	reg := newSchemaRegistry(schemasNode)

	pathsNode := getMapValue(top, "paths")
	if pathsNode == nil || pathsNode.Kind != yaml.MappingNode {
		return false, fmt.Errorf("missing or invalid 'paths' section")
	}

	changed := false

	for i := 0; i < len(pathsNode.Content); i += 2 {
		pathKey := pathsNode.Content[i]
		pathVal := pathsNode.Content[i+1]
		if pathVal.Kind != yaml.MappingNode {
			continue
		}

		for j := 0; j < len(pathVal.Content); j += 2 {
			methodKey := pathVal.Content[j]
			methodVal := pathVal.Content[j+1]
			method := methodKey.Value

			switch method {
			case "get", "post", "put", "delete", "patch", "options", "head":
			default:
				continue
			}

			if methodVal.Kind != yaml.MappingNode {
				continue
			}

			opIDNode := getMapValue(methodVal, "operationId")
			operationID := ""
			if opIDNode != nil && opIDNode.Kind == yaml.ScalarNode {
				operationID = opIDNode.Value
			}

			respNode := getMapValue(methodVal, "responses")
			if respNode == nil || respNode.Kind != yaml.MappingNode {
				continue
			}

			for k := 0; k < len(respNode.Content); k += 2 {
				codeKey := respNode.Content[k]
				codeVal := respNode.Content[k+1]

				changedHere, err := processResponseSchema(
					pathKey.Value, method, operationID, codeKey.Value,
					codeVal, schemasNode, reg, opts,
				)
				if err != nil {
					return false, err
				}
				if changedHere {
					changed = true
				}
			}
		}
	}

	return changed, nil
}

func processResponseSchema(
	path, method, operationID, status string,
	responseNode *yaml.Node,
	schemasNode *yaml.Node,
	reg *schemaRegistry,
	opts Options,
) (bool, error) {
	if responseNode.Kind != yaml.MappingNode {
		return false, nil
	}

	contentNode := getMapValue(responseNode, "content")
	if contentNode == nil || contentNode.Kind != yaml.MappingNode {
		return false, nil
	}

	var appJSON *yaml.Node
	for _, contentType := range []string{
		"application/json; charset=UTF-8",
		"application/json",
		"application/json; charset=utf-8",
	} {
		appJSON = getMapValue(contentNode, contentType)
		if appJSON != nil {
			break
		}
	}
	if appJSON == nil || appJSON.Kind != yaml.MappingNode {
		return false, nil
	}

	schemaNode := getMapValue(appJSON, "schema")
	if schemaNode == nil || schemaNode.Kind != yaml.MappingNode {
		return false, nil
	}

	if isRefOnlySchema(schemaNode) {
		return false, nil
	}

	allOfNode := getMapValue(schemaNode, "allOf")
	if allOfNode != nil && allOfNode.Kind == yaml.SequenceNode && len(allOfNode.Content) == 2 {
		baseNode := allOfNode.Content[0]
		extNode := allOfNode.Content[1]

		if isInlineObjectWithAlternatives(baseNode, extNode) {
			return handleAlternativesPattern(
				path, method, operationID, status,
				schemaNode, baseNode, schemasNode, reg, opts,
			)
		}
	}

	nameHint := deriveResponseSchemaNameHint(path, method, operationID, status)
	if opts.Verbose {
		fmt.Printf("Found inline schema at %s %s %s -> creating %s\n",
			method, path, status, nameHint)
	}

	_, changed, err := reg.componentizeSchema(schemaNode, nameHint, opts)
	return changed, err
}

func handleAlternativesPattern(
	path, method, operationID, status string,
	schemaNode, baseNode *yaml.Node,
	schemasNode *yaml.Node,
	reg *schemaRegistry,
	opts Options,
) (bool, error) {
	baseName := deriveSchemaName(operationID)
	if baseName == "" {
		nameHint := deriveResponseSchemaNameHint(path, method, operationID, status)
		if opts.Verbose {
			fmt.Printf("Found inline schema at %s %s %s -> creating %s\n",
				method, path, status, nameHint)
		}
		_, changed, err := reg.componentizeSchema(schemaNode, nameHint, opts)
		return changed, err
	}

	compositeName := baseName + "WithAlternatives"

	if opts.Verbose {
		fmt.Printf("Found inline allOf pattern at %s %s %s -> creating %s and %s\n",
			method, path, status, baseName, compositeName)
	}

	if _, exists := getMapValueExists(schemasNode, baseName); !exists {
		baseSchemaNode := cloneNode(baseNode)
		appendMapEntry(schemasNode, baseName, baseSchemaNode)
		reg.existingNames[baseName] = struct{}{}
		reg.byFingerprint[schemaFingerprint(baseSchemaNode)] = baseName
		if opts.Verbose {
			fmt.Printf("  Created components/schemas/%s\n", baseName)
		}
	}

	if _, exists := getMapValueExists(schemasNode, compositeName); !exists {
		compositeSchemaNode := buildCompositeSchema(baseName)
		appendMapEntry(schemasNode, compositeName, compositeSchemaNode)
		reg.existingNames[compositeName] = struct{}{}
		reg.byFingerprint[schemaFingerprint(compositeSchemaNode)] = compositeName
		if opts.Verbose {
			fmt.Printf("  Created components/schemas/%s\n", compositeName)
		}
	}

	makeRefOnlySchema(schemaNode, compositeName)

	return true, nil
}

func deriveResponseSchemaNameHint(path, method, operationID, status string) string {
	base := deriveSchemaName(operationID)
	if base == "" {
		base = deriveNameFromPathAndMethod(path, method)
	}
	if base == "" {
		base = "Response"
	}

	statusSuffix := status
	if statusSuffix == "" {
		statusSuffix = "Default"
	}

	return base + toPascalCase(statusSuffix) + "Response"
}

func deriveNameFromPathAndMethod(path, method string) string {
	parts := strings.Split(path, "/")
	var out []string
	for _, p := range parts {
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
			p = p[1 : len(p)-1]
		}
		out = append(out, toPascalCase(p))
	}
	out = append(out, toPascalCase(strings.ToLower(method)))
	return strings.Join(out, "")
}

// isInlineObjectWithAlternatives checks if we have the pattern:
// allOf[0] = inline object schema
// allOf[1] = inline object with properties.alternatives that is an array of same shape
func isInlineObjectWithAlternatives(baseNode, extNode *yaml.Node) bool {
	if baseNode.Kind != yaml.MappingNode || extNode.Kind != yaml.MappingNode {
		return false
	}

	// Check base is type: object
	baseType := getMapValue(baseNode, "type")
	if baseType == nil || baseType.Value != "object" {
		return false
	}

	// Check ext is type: object
	extType := getMapValue(extNode, "type")
	if extType == nil || extType.Value != "object" {
		return false
	}

	// Check ext has properties.alternatives
	extProps := getMapValue(extNode, "properties")
	if extProps == nil || extProps.Kind != yaml.MappingNode {
		return false
	}

	alts := getMapValue(extProps, "alternatives")
	if alts == nil || alts.Kind != yaml.MappingNode {
		return false
	}

	altType := getMapValue(alts, "type")
	if altType == nil || altType.Value != "array" {
		return false
	}

	items := getMapValue(alts, "items")
	if items == nil || items.Kind != yaml.MappingNode {
		return false
	}

	// items should be type: object (inline) with similar properties
	itemsType := getMapValue(items, "type")
	if itemsType == nil || itemsType.Value != "object" {
		return false
	}

	return true
}

// deriveSchemaName derives a schema name from the operationId.
// Examples:
//
//	get_airport       -> Airport
//	get_operator      -> Operator
//	get_airport_info  -> AirportInfo
//	post_user_data    -> UserData
func deriveSchemaName(operationID string) string {
	if operationID == "" {
		return ""
	}

	// Strip common HTTP verb prefixes
	lower := strings.ToLower(operationID)
	prefixes := []string{
		"get_", "post_", "put_", "delete_",
		"patch_", "options_", "head_",
		"list_", "create_", "update_", "remove_",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			operationID = operationID[len(p):]
			break
		}
	}

	if operationID == "" {
		return ""
	}

	// Convert snake_case / kebab-case to PascalCase
	return toPascalCase(operationID)
}

// toPascalCase converts a snake_case or kebab-case string to PascalCase.
func toPascalCase(s string) string {
	// Normalize separators
	s = strings.ReplaceAll(s, "-", "_")

	parts := strings.Split(s, "_")
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		// Uppercase first rune, keep the rest as-is
		runes := []rune(part)
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}

	return b.String()
}

// buildCompositeSchema creates the schema structure for XWithAlternatives.
func buildCompositeSchema(baseName string) *yaml.Node {
	// Build:
	// allOf:
	//   - $ref: '#/components/schemas/<baseName>'
	//   - type: object
	//     properties:
	//       alternatives:
	//         type: array
	//         description: An array of other possible matches
	//         items:
	//           $ref: '#/components/schemas/<baseName>'

	refValue := "#/components/schemas/" + baseName

	return &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			scalarNode("allOf"),
			{
				Kind: yaml.SequenceNode,
				Content: []*yaml.Node{
					// First element: $ref to base
					{
						Kind: yaml.MappingNode,
						Content: []*yaml.Node{
							scalarNode("$ref"),
							scalarNode(refValue),
						},
					},
					// Second element: object with alternatives
					{
						Kind: yaml.MappingNode,
						Content: []*yaml.Node{
							scalarNode("type"),
							scalarNode("object"),
							scalarNode("properties"),
							{
								Kind: yaml.MappingNode,
								Content: []*yaml.Node{
									scalarNode("alternatives"),
									{
										Kind: yaml.MappingNode,
										Content: []*yaml.Node{
											scalarNode("type"),
											scalarNode("array"),
											scalarNode("description"),
											scalarNode("An array of other possible matches"),
											scalarNode("items"),
											{
												Kind: yaml.MappingNode,
												Content: []*yaml.Node{
													scalarNode("$ref"),
													scalarNode(refValue),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// YAML node helpers

func getMapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(m.Content); i += 2 {
		k := m.Content[i]
		v := m.Content[i+1]
		if k.Value == key {
			return v
		}
	}
	return nil
}

func getMapValueExists(m *yaml.Node, key string) (*yaml.Node, bool) {
	v := getMapValue(m, key)
	return v, v != nil
}

func ensureMapValue(m *yaml.Node, key string) *yaml.Node {
	if m.Kind != yaml.MappingNode {
		m.Kind = yaml.MappingNode
		m.Content = nil
	}
	if v := getMapValue(m, key); v != nil {
		return v
	}
	k := scalarNode(key)
	v := &yaml.Node{Kind: yaml.MappingNode}
	m.Content = append(m.Content, k, v)
	return v
}

func appendMapEntry(m *yaml.Node, key string, value *yaml.Node) {
	k := scalarNode(key)
	m.Content = append(m.Content, k, value)
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: value,
	}
}

func cloneNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	out := *n
	out.Content = make([]*yaml.Node, len(n.Content))
	for i, c := range n.Content {
		out.Content[i] = cloneNode(c)
	}
	return &out
}

func isRefOnlySchema(schema *yaml.Node) bool {
	if schema.Kind != yaml.MappingNode {
		return false
	}
	if len(schema.Content) != 2 {
		return false
	}
	k := schema.Content[0]
	return k.Value == "$ref"
}

func makeRefOnlySchema(schema *yaml.Node, compositeName string) {
	schema.Content = schema.Content[:0]
	key := scalarNode("$ref")
	val := scalarNode("#/components/schemas/" + compositeName)
	schema.Content = append(schema.Content, key, val)
}
