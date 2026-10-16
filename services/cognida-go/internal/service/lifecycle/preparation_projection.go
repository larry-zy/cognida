package lifecycle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	d "cognida/internal/model/lifecycle"
)

func projectionField(name string) bool {
	return name != "" && name == strings.TrimSpace(name) && utf8.RuneCountInString(name) <= 200 && !strings.ContainsAny(name, "\x00\r\n")
}

func projectedDocument(raw json.RawMessage, field string) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return "", d.ErrInvalid
	}
	seen := map[string]bool{}
	var text string
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return "", d.ErrInvalid
		}
		seen[key] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return "", d.ErrInvalid
		}
		if key == field && json.Unmarshal(value, &text) != nil {
			return "", d.ErrInvalid
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') || decoder.Decode(new(any)) != io.EOF || strings.TrimSpace(text) == "" {
		return "", d.ErrInvalid
	}
	return text, nil
}

// projectKnowledgeText preserves every selected document and rejects an entire
// operation on malformed/empty input. It never fills answers or selects other
// table columns; source identity is rendered alongside each document.
func projectKnowledgeText(input d.Input, projection d.PreparationTextProjection, normalize func(string) string) (d.Input, error) {
	column, idColumn := -1, -1
	for i, name := range input.Columns {
		if name == projection.Column {
			column = i
		}
		if name == projection.IDColumn {
			idColumn = i
		}
	}
	if column < 0 || (projection.IDColumn != "" && idColumn < 0) || len(input.Rows) > 256 {
		return d.Input{}, fmt.Errorf("%w: unknown source column or more than 256 source rows", d.ErrInvalid)
	}
	var output strings.Builder
	documents := 0
	for rowIndex, row := range input.Rows {
		encoded, ok := row[column].(string)
		if !ok {
			return d.Input{}, fmt.Errorf("%w: document column must contain a JSON array string", d.ErrInvalid)
		}
		var items []json.RawMessage
		decoder := json.NewDecoder(strings.NewReader(encoded))
		if decoder.Decode(&items) != nil || decoder.Decode(new(any)) != io.EOF || len(items) < 1 || documents+len(items) > 256 {
			return d.Input{}, fmt.Errorf("%w: require 1-256 source documents in total", d.ErrInvalid)
		}
		identity := ""
		if idColumn >= 0 {
			switch id := row[idColumn].(type) {
			case string:
				identity = id
			case json.Number:
				identity = id.String()
			default:
				return d.Input{}, fmt.Errorf("%w: source identity must be text or an exact JSON number", d.ErrInvalid)
			}
			if strings.TrimSpace(identity) == "" || utf8.RuneCountInString(identity) > 512 || strings.ContainsRune(identity, '\x00') {
				return d.Input{}, d.ErrInvalid
			}
		}
		for index, item := range items {
			text, err := projectedDocument(item, projection.TextField)
			if err != nil {
				return d.Input{}, fmt.Errorf("%w: source row %d document %d has invalid text", d.ErrInvalid, rowIndex+1, index+1)
			}
			if output.Len() > 0 {
				output.WriteString("\n\n")
			}
			fmt.Fprintf(&output, "[Source row %d", rowIndex+1)
			if idColumn >= 0 {
				quoted, _ := json.Marshal(identity)
				fmt.Fprintf(&output, ", id %s", quoted)
			}
			fmt.Fprintf(&output, ", document %d]\n", index+1)
			output.WriteString(normalize(text))
			documents++
			if output.Len() > 800000 {
				return d.Input{}, fmt.Errorf("%w: projected text limit", d.ErrInvalid)
			}
		}
	}
	out := d.Input{Kind: "text", Text: output.String()}
	return out, out.Validate()
}
