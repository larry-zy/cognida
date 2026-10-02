package governance

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"reflect"
	"strings"
)

// JSONEqual compares values independently of database JSON formatting and key
// ordering, without losing integer precision through float64 conversion.
func JSONEqual(a, b json.RawMessage) bool {
	decode := func(raw json.RawMessage) (any, error) {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return nil, ErrInvalid
		}
		return normalizeJSON(v), nil
	}
	x, err := decode(a)
	if err != nil {
		return false
	}
	y, err := decode(b)
	return err == nil && reflect.DeepEqual(x, y)
}

type jsonNumber string

func normalizeJSON(v any) any {
	switch x := v.(type) {
	case json.Number:
		// Normalize coefficient/exponent without constructing powers of ten. A
		// valid JSON exponent such as 1e999999999 must not allocate a huge big.Rat.
		raw := string(x)
		sign := ""
		if strings.HasPrefix(raw, "-") {
			sign = "-"
			raw = raw[1:]
		}
		exponent := new(big.Int)
		if at := strings.IndexAny(raw, "eE"); at >= 0 {
			exponent.SetString(raw[at+1:], 10)
			raw = raw[:at]
		}
		if at := strings.IndexByte(raw, '.'); at >= 0 {
			exponent.Sub(exponent, big.NewInt(int64(len(raw)-at-1)))
			raw = raw[:at] + raw[at+1:]
		}
		raw = strings.TrimLeft(raw, "0")
		if raw == "" {
			return jsonNumber("0")
		}
		trimmed := strings.TrimRight(raw, "0")
		exponent.Add(exponent, big.NewInt(int64(len(raw)-len(trimmed))))
		return jsonNumber(sign + trimmed + "e" + exponent.String())
	case []any:
		for i := range x {
			x[i] = normalizeJSON(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = normalizeJSON(x[k])
		}
	}
	return v
}
