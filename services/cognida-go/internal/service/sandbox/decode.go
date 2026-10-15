package sandbox

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"

	d "cognida/internal/model/sandbox"
)

const MaxSubmissionBytes = 64 << 10

var jsonFieldName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// DecodeSubmitRequest rejects unknown and duplicate fields at every level,
// including attempted credential, mount, socket and profile-policy overrides.
// It is not an authorization check; always follow with ValidateSubmission.
func DecodeSubmitRequest(raw []byte) (d.SubmitRequest, error) {
	var req d.SubmitRequest
	if len(raw) == 0 || len(raw) > MaxSubmissionBytes {
		return req, invalid("request size exceeds contract")
	}
	check := json.NewDecoder(bytes.NewReader(raw))
	check.UseNumber()
	if err := checkJSONValue(check, 0); err != nil {
		return req, invalid("invalid or ambiguous JSON request")
	}
	if _, err := check.Token(); err != io.EOF {
		return req, invalid("exactly one JSON request required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return req, invalid("unknown or invalid request fields")
	}
	if req.SchemaVersion == "" {
		return req, invalid("versioned request object required")
	}
	return req, nil
}

func checkJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 8 {
		return d.ErrInvalid
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			// encoding/json matches struct fields case-insensitively. Require
			// canonical names so aliases cannot bypass duplicate detection.
			if !ok || !jsonFieldName.MatchString(name) || keys[name] {
				return d.ErrInvalid
			}
			keys[name] = true
			if err := checkJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := checkJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return d.ErrInvalid
	}
	_, err = decoder.Token()
	return err
}
