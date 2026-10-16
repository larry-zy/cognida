package apisource

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	d "cognida/internal/model/apisource"
	l "cognida/internal/model/lifecycle"
)

var errSnapshot = errors.New("API source snapshot changed")
var errLimit = errors.New("API source limit reached before completion")
var errRecord = errors.New("API source record shape or stable key conflict")

func pointer(root any, path string) (any, bool) {
	if path == "" {
		return root, true
	}
	value := root
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		m, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return value, true
}
func decode(raw []byte) (any, error) {
	var out any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if e := decoder.Decode(&out); e != nil {
		return nil, errRecord
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errRecord
	}
	return out, nil
}
func (s *Service) pageURL(run *d.Run) (string, error) {
	u, e := url.Parse(run.Config.URL)
	if e != nil {
		return "", d.ErrInvalid
	}
	q := u.Query()
	p := run.Config.Pagination
	if p.Mode == "page" {
		q.Set(p.Parameter, strconv.Itoa(len(run.Pages)+1))
	}
	if p.Mode == "cursor" && run.CursorCiphertext != "" {
		v, e := s.cipher.Decrypt(run.CursorCiphertext)
		if e != nil {
			return "", d.ErrForbidden
		}
		q.Set(p.Parameter, v)
	}
	if p.PageSizeParameter != "" {
		q.Set(p.PageSizeParameter, strconv.Itoa(p.PageSize))
	}
	if run.SnapshotCiphertext != "" {
		v, e := s.cipher.Decrypt(run.SnapshotCiphertext)
		if e != nil {
			return "", d.ErrForbidden
		}
		q.Set(p.SnapshotParameter, v)
	}
	if run.WatermarkBefore != "" {
		q.Set(run.Config.WatermarkParameter, run.WatermarkBefore)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func (s *Service) parsePage(run *d.Run, raw []byte) (d.Checkpoint, error) {
	root, e := decode(raw)
	if e != nil {
		return d.Checkpoint{}, e
	}
	value, ok := pointer(root, run.Config.ItemsPointer)
	if !ok {
		return d.Checkpoint{}, errRecord
	}
	items, ok := value.([]any)
	if !ok {
		return d.Checkpoint{}, errRecord
	}
	cp := d.Checkpoint{ExpectedIndex: len(run.Pages), Records: append([]json.RawMessage{}, run.Records...), Keys: map[string]string{}, DuplicateCount: run.DuplicateCount, DeletedCount: run.DeletedCount, SnapshotCiphertext: run.SnapshotCiphertext, SnapshotDigest: run.SnapshotDigest, WatermarkAfter: run.WatermarkAfter}
	for key, value := range run.Keys {
		cp.Keys[key] = value
	}
	p := run.Config.Pagination
	if p.SnapshotPointer != "" {
		value, ok := pointer(root, p.SnapshotPointer)
		token, isString := value.(string)
		if !ok || !isString || token == "" || len(token) > 4096 {
			return cp, errSnapshot
		}
		digest := hash([]byte(token))
		if cp.SnapshotDigest != "" && digest != cp.SnapshotDigest {
			return cp, errSnapshot
		}
		if cp.SnapshotDigest == "" {
			cp.SnapshotCiphertext, e = s.cipher.Encrypt(token)
			if e != nil {
				return cp, d.ErrInvalid
			}
			cp.SnapshotDigest = digest
		}
	}
	cp.Complete = p.Mode == "none"
	if p.Mode == "page" {
		value, ok := pointer(root, p.NextPointer)
		more, isBool := value.(bool)
		if !ok || !isBool {
			return cp, errRecord
		}
		cp.Complete = !more
	}
	if p.Mode == "cursor" {
		value, ok := pointer(root, p.NextPointer)
		if !ok {
			return cp, errRecord
		}
		if value == nil || value == "" {
			cp.Complete = true
		} else {
			next, isString := value.(string)
			if !isString || len(next) > 4096 {
				return cp, errRecord
			}
			if run.CursorCiphertext != "" {
				prior, e := s.cipher.Decrypt(run.CursorCiphertext)
				if e != nil {
					return cp, d.ErrForbidden
				}
				if prior == next {
					return cp, errSnapshot
				}
			}
			cp.CursorCiphertext, e = s.cipher.Encrypt(next)
			if e != nil {
				return cp, d.ErrInvalid
			}
		}
	}
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok || len(record) == 0 || len(record) > 200 {
			return cp, errRecord
		}
		key, ok := record[run.Config.RecordKey]
		if !ok {
			return cp, errRecord
		}
		switch v := key.(type) {
		case string:
			if v == "" || len(v) > 1024 {
				return cp, errRecord
			}
		case json.Number:
			if len(v) > 128 {
				return cp, errRecord
			}
		default:
			return cp, errRecord
		}
		encoded, e := json.Marshal(record)
		if e != nil {
			return cp, errRecord
		}
		encodedKey, _ := json.Marshal(key)
		keyHash, digest := hash(encodedKey), hash(encoded)
		if prior, exists := cp.Keys[keyHash]; exists {
			if prior != digest {
				return cp, errRecord
			}
			cp.DuplicateCount++
			continue
		}
		cp.Keys[keyHash] = digest
		if field := run.Config.DeletedField; field != "" {
			if deleted, exists := record[field]; exists {
				flag, ok := deleted.(bool)
				if !ok {
					return cp, errRecord
				}
				if flag {
					cp.DeletedCount++
				}
			}
		}
		if field := run.Config.WatermarkField; field != "" {
			stamp, ok := record[field].(string)
			if !ok {
				return cp, errRecord
			}
			at, e := time.Parse(time.RFC3339Nano, stamp)
			if e != nil {
				return cp, errRecord
			}
			before, _ := time.Parse(time.RFC3339Nano, run.WatermarkBefore)
			if !before.IsZero() && at.Before(before) {
				return cp, errSnapshot
			}
			current, _ := time.Parse(time.RFC3339Nano, cp.WatermarkAfter)
			if at.After(current) {
				cp.WatermarkAfter = at.UTC().Format(time.RFC3339Nano)
			}
		}
		cp.Records = append(cp.Records, encoded)
		if len(cp.Records) > run.Config.Limits.MaxRecords {
			return cp, errLimit
		}
	}
	all, _ := json.Marshal(cp.Records)
	if len(all) > d.MaxNormalizedBytes {
		return cp, errLimit
	}
	cp.Page = d.PageReceipt{Number: len(run.Pages) + 1, Digest: hash(raw), Bytes: int64(len(raw)), Records: len(items), AcquiredAt: time.Now().UTC()}
	return cp, nil
}
func table(records []json.RawMessage) (l.Input, error) {
	rows := make([]map[string]any, 0, len(records))
	columns := map[string]bool{}
	types := map[string]string{}
	for _, raw := range records {
		value, e := decode(raw)
		if e != nil {
			return l.Input{}, e
		}
		row, ok := value.(map[string]any)
		if !ok {
			return l.Input{}, errRecord
		}
		rows = append(rows, row)
		for key, value := range row {
			columns[key] = true
			typ := "VARCHAR"
			switch value.(type) {
			case nil:
				continue
			case json.Number:
				typ = "NUMERIC"
			case bool:
				typ = "BOOLEAN"
			case map[string]any, []any:
				typ = "JSON"
			}
			if prior := types[key]; prior != "" && prior != typ {
				types[key] = "VARCHAR"
			} else {
				types[key] = typ
			}
		}
	}
	names := make([]string, 0, len(columns))
	for name := range columns {
		names = append(names, name)
	}
	sort.Strings(names)
	input := l.Input{Kind: "table", Columns: names, ColumnTypes: types, Rows: [][]any{}}
	for _, row := range rows {
		cells := make([]any, len(names))
		for i, name := range names {
			value := row[name]
			switch v := value.(type) {
			case json.Number:
				cells[i] = v.String()
			case map[string]any, []any:
				raw, _ := json.Marshal(v)
				cells[i] = string(raw)
			default:
				cells[i] = value
			}
		}
		input.Rows = append(input.Rows, cells)
	}
	if e := input.Validate(); e != nil {
		return l.Input{}, fmt.Errorf("normalized API table exceeds lifecycle limits: %w", d.ErrInvalid)
	}
	return input, nil
}
