package governedops

import (
	"context"
	"errors"
	"strings"
	"unicode"

	agentctx "cognida/internal/model/agent"
	d "cognida/internal/model/governedops"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/service/lifecycle"
)

// Literal SQL is a compatibility input only. It is parsed into the same typed
// plan as HTTP. No original statement is executed and no confirmation is implied.
type sqlToken struct {
	text    string
	literal bool
}

func tokenizeSQL(s string) ([]sqlToken, error) {
	if len(s) == 0 || len(s) > 32<<10 {
		return nil, d.ErrInvalid
	}
	out := []sqlToken{}
	for i := 0; i < len(s); {
		if s[i] == ' ' || s[i] == '\n' || s[i] == '\r' || s[i] == '\t' {
			i++
			continue
		}
		if s[i] == '\'' {
			i++
			var b strings.Builder
			closed := false
			for i < len(s) {
				if s[i] == '\'' {
					if i+1 < len(s) && s[i+1] == '\'' {
						b.WriteByte('\'')
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				if s[i] == '\\' || s[i] == 0 {
					return nil, d.ErrInvalid
				}
				b.WriteByte(s[i])
				i++
			}
			if !closed {
				return nil, d.ErrInvalid
			}
			out = append(out, sqlToken{b.String(), true})
			continue
		}
		if s[i] == '`' {
			start := i + 1
			i++
			for i < len(s) && s[i] != '`' {
				i++
			}
			if i == len(s) || !d.Identifier(s[start:i]) {
				return nil, d.ErrInvalid
			}
			out = append(out, sqlToken{s[start:i], false})
			i++
			continue
		}
		if strings.ContainsRune("(),=;", rune(s[i])) {
			out = append(out, sqlToken{string(s[i]), false})
			i++
			continue
		}
		start := i
		for i < len(s) && !unicode.IsSpace(rune(s[i])) && !strings.ContainsRune("(),=;'`", rune(s[i])) {
			i++
		}
		if start == i {
			return nil, d.ErrInvalid
		}
		out = append(out, sqlToken{s[start:i], false})
	}
	if len(out) > 256 {
		return nil, d.ErrInvalid
	}
	return out, nil
}

type literalParser struct {
	tokens []sqlToken
	i      int
}

func (p *literalParser) take(word string) bool {
	if p.i < len(p.tokens) && !p.tokens[p.i].literal && strings.EqualFold(p.tokens[p.i].text, word) {
		p.i++
		return true
	}
	return false
}
func (p *literalParser) ident() (string, error) {
	if p.i == len(p.tokens) || p.tokens[p.i].literal || !d.Identifier(p.tokens[p.i].text) {
		return "", d.ErrInvalid
	}
	v := p.tokens[p.i].text
	p.i++
	return v, nil
}
func (p *literalParser) cell() (d.Cell, error) {
	if p.i == len(p.tokens) {
		return d.Cell{}, d.ErrInvalid
	}
	t := p.tokens[p.i]
	p.i++
	c := d.Cell{Type: "number", Text: t.text}
	if t.literal {
		c.Type = "text"
	} else {
		switch strings.ToUpper(t.text) {
		case "NULL":
			c = d.Cell{Type: "null"}
		case "TRUE", "FALSE":
			c = d.Cell{Type: "boolean", Text: strings.ToLower(t.text)}
		}
	}
	return c, c.Validate()
}
func (p *literalParser) assignments(separator string) (map[string]d.Cell, error) {
	out := map[string]d.Cell{}
	for {
		key, e := p.ident()
		if e != nil || !p.take("=") {
			return nil, d.ErrInvalid
		}
		if _, ok := out[key]; ok {
			return nil, d.ErrInvalid
		}
		value, e := p.cell()
		if e != nil {
			return nil, e
		}
		out[key] = value
		if !p.take(separator) {
			return out, nil
		}
	}
}
func parseLiteralSQL(raw string) (d.Request, error) {
	tokens, e := tokenizeSQL(raw)
	if e != nil {
		return d.Request{}, e
	}
	p := literalParser{tokens: tokens}
	r := d.Request{ExpectedVersion: 1}
	switch {
	case p.take("INSERT"):
		r.Action = "insert"
		if !p.take("INTO") {
			return r, d.ErrInvalid
		}
		r.Target, e = p.ident()
		if e != nil || !p.take("(") {
			return r, d.ErrInvalid
		}
		columns := []string{}
		seen := map[string]bool{}
		for {
			c, e := p.ident()
			if e != nil || seen[strings.ToLower(c)] {
				return r, d.ErrInvalid
			}
			seen[strings.ToLower(c)] = true
			columns = append(columns, c)
			if !p.take(",") {
				break
			}
		}
		if !p.take(")") || !p.take("VALUES") || !p.take("(") {
			return r, d.ErrInvalid
		}
		r.Values = map[string]d.Cell{}
		for i, c := range columns {
			if i > 0 && !p.take(",") {
				return r, d.ErrInvalid
			}
			v, e := p.cell()
			if e != nil {
				return r, e
			}
			r.Values[c] = v
		}
		if !p.take(")") {
			return r, d.ErrInvalid
		}
	case p.take("UPDATE"):
		r.Action = "update"
		r.Target, e = p.ident()
		if e != nil || !p.take("SET") {
			return r, d.ErrInvalid
		}
		r.Values, e = p.assignments(",")
		if e != nil || !p.take("WHERE") {
			return r, d.ErrInvalid
		}
		r.Where, e = p.assignments("AND")
		if e != nil {
			return r, e
		}
	case p.take("DELETE"):
		r.Action = "delete"
		if !p.take("FROM") {
			return r, d.ErrInvalid
		}
		r.Target, e = p.ident()
		if e != nil || !p.take("WHERE") {
			return r, d.ErrInvalid
		}
		r.Where, e = p.assignments("AND")
		if e != nil {
			return r, e
		}
	default:
		return r, d.ErrInvalid
	}
	p.take(";")
	if p.i != len(tokens) {
		return r, d.ErrInvalid
	}
	return r, r.Validate()
}

// WithReadQuery must be wired to the existing authenticated read-only SQL tool.
// It returns a real owned ResultStore handle; it never executes mutation SQL.
func (s *Service) WithReadQuery(read func(context.Context, string) (string, error)) *Service {
	s.readQuery = read
	return s
}
func (s *Service) agentPrior(ctx context.Context, p l.Principal, key, fingerprint string) (*d.Job, error) {
	if e := s.auth(ctx, p, "admin"); e != nil {
		return nil, e
	}
	if !validKey(key) {
		return nil, d.ErrInvalid
	}
	prior, e := s.repo.FindByKey(ctx, p, d.Digest(key))
	if e != nil || prior == nil {
		return prior, e
	}
	if prior.Plan.Request.SourceQueryHash != fingerprint {
		return nil, d.ErrConflict
	}
	return s.Read(ctx, p, prior.ID)
}
func (s *Service) PrepareAgentSQL(ctx context.Context, p l.Principal, sql, key string) (*d.Job, error) {
	req, e := parseLiteralSQL(sql)
	if e != nil {
		return nil, e
	}
	fingerprint := d.Digest([]any{"sql", sql})
	prior, e := s.agentPrior(ctx, p, key, fingerprint)
	if e != nil || prior != nil {
		return prior, e
	}
	target, e := s.repo.Target(ctx, p, req.Target)
	if e != nil {
		return nil, e
	}
	req.ExpectedVersion = target.Version
	req.SourceQueryHash = fingerprint
	return s.Prepare(ctx, p, req, key)
}
func (s *Service) PrepareAgentETL(ctx context.Context, p l.Principal, target, resultID, sql, key string, replace bool) (*d.Job, error) {
	if !d.Identifier(target) || (resultID == "") == (strings.TrimSpace(sql) == "") {
		return nil, d.ErrInvalid
	}
	fingerprint := d.Digest([]any{"etl", target, resultID, sql, replace})
	prior, e := s.agentPrior(ctx, p, key, fingerprint)
	if e != nil || prior != nil {
		return prior, e
	}
	expected := int64(0)
	current, e := s.repo.Target(ctx, p, target)
	if e == nil {
		if !replace && current.Version > 0 {
			return nil, d.ErrConflict
		}
		expected = current.Version
	} else if !errors.Is(e, d.ErrNotFound) {
		return nil, e
	}
	if resultID == "" {
		if s.readQuery == nil {
			return nil, d.ErrInvalid
		}
		resultID, e = s.readQuery(ctx, sql)
		if e != nil {
			return nil, e
		}
		if resultID == "" {
			return nil, d.ErrInvalid
		}
	}
	ref, e := s.assets.Acquire(ctx, p, lifecycle.SourceRequest{Kind: "result", ResultID: resultID, SessionID: agentctx.MustGetSessionID(ctx)}, "ETL input", "operation-input:"+d.Digest([]any{p.TenantID, p.UserID, key, fingerprint}))
	if e != nil {
		return nil, e
	}
	return s.Prepare(ctx, p, d.Request{Action: "etl", Target: target, ExpectedVersion: expected, InputRef: &ref.RefID, SourceQueryHash: fingerprint}, key)
}
