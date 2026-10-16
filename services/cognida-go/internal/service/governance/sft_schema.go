package governance

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"

	d "cognida/internal/model/governance"
)

// SFT tool parameters use a deliberately bounded, local JSON Schema subset.
// Unsupported keywords (including references/combinators) fail closed. No schema
// is fetched and no argument is coerced or rewritten in the exported sample.
type sftSchema struct {
	kind       string
	properties map[string]*sftSchema
	required   map[string]bool
	additional bool
	extra      *sftSchema
	items      *sftSchema
	enum       []any // Scalar enums only; numeric equality is exact.
	minSize    *int
	maxSize    *int
	minimum    *big.Rat
	maximum    *big.Rat
}

func sftSchemaError() error {
	return fmt.Errorf("%w: unsupported or invalid SFT tool parameter schema", d.ErrInvalid)
}
func sftArgumentsError() error {
	return fmt.Errorf("%w: SFT tool arguments violate declared parameters", d.ErrInvalid)
}

func compileSFTSchema(raw json.RawMessage) (*sftSchema, error) {
	var value any
	if len(raw) > 64<<10 || sftDecode(string(raw), &value) != nil {
		return nil, sftSchemaError()
	}
	nodes := 0
	s, err := compileSFTSchemaNode(value, 0, &nodes)
	if err != nil || s.kind != "object" {
		return nil, sftSchemaError()
	}
	return s, nil
}

func compileSFTSchemaNode(value any, depth int, nodes *int) (*sftSchema, error) {
	*nodes++
	fields, ok := value.(map[string]any)
	if !ok || fields == nil || depth > 12 || *nodes > 256 {
		return nil, sftSchemaError()
	}
	s := &sftSchema{properties: map[string]*sftSchema{}, required: map[string]bool{}, additional: true}
	s.kind, _ = fields["type"].(string)
	switch s.kind {
	case "object", "array", "string", "number", "integer", "boolean", "null":
	default:
		return nil, sftSchemaError()
	}
	for key, v := range fields {
		switch key {
		case "type":
		case "title", "description":
			text, ok := v.(string)
			if !ok || len(text) > 8000 {
				return nil, sftSchemaError()
			}
		case "properties":
			props, ok := v.(map[string]any)
			if s.kind != "object" || !ok || props == nil || len(props) > 100 {
				return nil, sftSchemaError()
			}
			for name, raw := range props {
				if len(name) == 0 || len(name) > 256 {
					return nil, sftSchemaError()
				}
				child, err := compileSFTSchemaNode(raw, depth+1, nodes)
				if err != nil {
					return nil, err
				}
				s.properties[name] = child
			}
		case "required":
			list, ok := v.([]any)
			if s.kind != "object" || !ok || len(list) > 100 {
				return nil, sftSchemaError()
			}
			for _, item := range list {
				name, ok := item.(string)
				if !ok || name == "" || s.required[name] {
					return nil, sftSchemaError()
				}
				s.required[name] = true
			}
		case "additionalProperties":
			if s.kind != "object" {
				return nil, sftSchemaError()
			}
			if allow, ok := v.(bool); ok {
				s.additional = allow
			} else {
				child, err := compileSFTSchemaNode(v, depth+1, nodes)
				if err != nil {
					return nil, err
				}
				s.extra = child
			}
		case "items":
			if s.kind != "array" {
				return nil, sftSchemaError()
			}
			child, err := compileSFTSchemaNode(v, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			s.items = child
		case "enum":
			list, ok := v.([]any)
			if !ok || len(list) == 0 || len(list) > 100 || s.kind == "object" || s.kind == "array" {
				return nil, sftSchemaError()
			}
			for i, item := range list {
				if !s.scalarType(item) {
					return nil, sftSchemaError()
				}
				for _, prior := range list[:i] {
					if sftScalarEqual(prior, item) {
						return nil, sftSchemaError()
					}
				}
			}
			s.enum = list
		case "minLength", "maxLength", "minItems", "maxItems":
			limit := 100000
			if strings.HasSuffix(key, "Items") {
				limit = 1000
				if s.kind != "array" {
					return nil, sftSchemaError()
				}
			} else if s.kind != "string" {
				return nil, sftSchemaError()
			}
			n, ok := v.(json.Number)
			i, err := strconv.Atoi(string(n))
			if !ok || err != nil || i < 0 || i > limit {
				return nil, sftSchemaError()
			}
			if strings.HasPrefix(key, "min") {
				s.minSize = &i
			} else {
				s.maxSize = &i
			}
		case "minimum", "maximum":
			if s.kind != "number" && s.kind != "integer" {
				return nil, sftSchemaError()
			}
			n, ok := v.(json.Number)
			r, valid := sftNumber(n)
			if !ok || !valid {
				return nil, sftSchemaError()
			}
			if key == "minimum" {
				s.minimum = r
			} else {
				s.maximum = r
			}
		default:
			return nil, sftSchemaError()
		}
	}
	for name := range s.required {
		if s.properties[name] == nil {
			return nil, sftSchemaError()
		}
	}
	if s.kind == "array" && s.items == nil || s.minSize != nil && s.maxSize != nil && *s.minSize > *s.maxSize || s.minimum != nil && s.maximum != nil && s.minimum.Cmp(s.maximum) > 0 {
		return nil, sftSchemaError()
	}
	return s, nil
}

func sftNumber(n json.Number) (*big.Rat, bool) {
	text := string(n)
	if len(text) == 0 || len(text) > 256 {
		return nil, false
	}
	if i := strings.IndexAny(text, "eE"); i >= 0 {
		exponent, err := strconv.Atoi(text[i+1:])
		if err != nil || exponent < -1000 || exponent > 1000 {
			return nil, false
		}
	}
	return new(big.Rat).SetString(text)
}
func (s *sftSchema) scalarType(value any) bool {
	switch s.kind {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	case "number", "integer":
		n, ok := value.(json.Number)
		r, valid := sftNumber(n)
		return ok && valid && (s.kind == "number" || r.IsInt())
	}
	return false
}
func sftScalarEqual(a, b any) bool {
	if x, ok := a.(json.Number); ok {
		y, yes := b.(json.Number)
		xn, xv := sftNumber(x)
		yn, yv := sftNumber(y)
		return yes && xv && yv && xn.Cmp(yn) == 0
	}
	return a == b // Both values are checked scalars.
}

func (s *sftSchema) validate(value any) bool {
	switch s.kind {
	case "object":
		object, ok := value.(map[string]any)
		if !ok || object == nil {
			return false
		}
		for name := range s.required {
			if _, found := object[name]; !found {
				return false
			}
		}
		for name, value := range object {
			child := s.properties[name]
			if child == nil {
				child = s.extra
				if child == nil && !s.additional {
					return false
				}
			}
			if child != nil && !child.validate(value) {
				return false
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok || !s.sizeAllowed(len(array)) {
			return false
		}
		for _, item := range array {
			if !s.items.validate(item) {
				return false
			}
		}
	default:
		if !s.scalarType(value) {
			return false
		}
		if text, ok := value.(string); ok && !s.sizeAllowed(utf8.RuneCountInString(text)) {
			return false
		}
		if n, ok := value.(json.Number); ok {
			r, _ := sftNumber(n)
			if s.minimum != nil && r.Cmp(s.minimum) < 0 || s.maximum != nil && r.Cmp(s.maximum) > 0 {
				return false
			}
		}
	}
	if s.enum != nil {
		for _, option := range s.enum {
			if sftScalarEqual(option, value) {
				return true
			}
		}
		return false
	}
	return true
}
func (s *sftSchema) sizeAllowed(size int) bool {
	return (s.minSize == nil || size >= *s.minSize) && (s.maxSize == nil || size <= *s.maxSize)
}

func boundedSFTArguments(value any, depth int, nodes *int) bool {
	*nodes++
	if depth > 32 || *nodes > 10000 {
		return false
	}
	switch v := value.(type) {
	case map[string]any:
		if len(v) > 1000 {
			return false
		}
		for _, item := range v {
			if !boundedSFTArguments(item, depth+1, nodes) {
				return false
			}
		}
	case []any:
		if len(v) > 1000 {
			return false
		}
		for _, item := range v {
			if !boundedSFTArguments(item, depth+1, nodes) {
				return false
			}
		}
	case json.Number:
		_, ok := sftNumber(v)
		return ok
	}
	return true
}
