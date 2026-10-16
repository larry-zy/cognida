// Package huggingface imports immutable static Hub files as independent raw assets.
package huggingface

import (
	"bytes"
	a "cognida/internal/model/apisource"
	d "cognida/internal/model/huggingface"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/service/collection/fetcher"
	"cognida/internal/service/datasource"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Assets interface {
	ImportDerived(context.Context, l.Principal, l.Kind, string, []l.RefID, json.RawMessage, l.PolicySnapshot) (l.DataAssetRef, error)
	Resolve(context.Context, int64, int64, l.RefID) (l.DataAssetRef, json.RawMessage, error)
}
type HubFetcher interface {
	FetchHuggingFace(context.Context, string, fetcher.JSONCredential, int64) (*fetcher.Result, error)
}
type Decoder interface {
	Decode(context.Context, []byte, string, string, int, d.FieldMapping) (d.DecodeResponse, error)
}
type CredentialProvider interface {
	Resolve(context.Context, l.Principal, string) (fetcher.JSONCredential, error)
}
type Authorizer func(context.Context, l.Principal, string) error
type Service struct {
	repo        d.Repository
	assets      Assets
	credentials CredentialProvider
	fetch       HubFetcher
	decoder     Decoder
	authorize   Authorizer
}

func New(repo d.Repository, assets Assets, credentials CredentialProvider, fetch HubFetcher, decoder Decoder, authorize Authorizer) (*Service, error) {
	if repo == nil || assets == nil || fetch == nil || authorize == nil {
		return nil, d.ErrUnavailable
	}
	return &Service{repo, assets, credentials, fetch, decoder, authorize}, nil
}

type credentialRepository interface {
	Credential(context.Context, l.Principal, string) (*a.Credential, error)
}
type managedCredentials struct {
	repo   credentialRepository
	cipher *datasource.Cipher
}

func NewManagedCredentials(repo credentialRepository, cipher *datasource.Cipher) CredentialProvider {
	return &managedCredentials{repo, cipher}
}
func (m *managedCredentials) Resolve(ctx context.Context, p l.Principal, id string) (fetcher.JSONCredential, error) {
	if p.Validate() != nil || id == "" || m.repo == nil || m.cipher == nil {
		return fetcher.JSONCredential{}, d.ErrForbidden
	}
	c, e := m.repo.Credential(ctx, p, id)
	if e != nil || c == nil || !c.Enabled || c.Kind != "bearer" || c.Origin != "https://huggingface.co/" {
		return fetcher.JSONCredential{}, d.ErrForbidden
	}
	secret, e := m.cipher.Decrypt(c.Ciphertext)
	if e != nil {
		return fetcher.JSONCredential{}, d.ErrForbidden
	}
	return fetcher.JSONCredential{Origin: c.Origin, Header: "Authorization", Value: "Bearer " + secret}, nil
}
func (s *Service) auth(ctx context.Context, p l.Principal, action string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if p.Validate() != nil || s.authorize == nil {
		return d.ErrForbidden
	}
	if e := s.authorize(ctx, p, action); e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return d.ErrForbidden
	}
	return nil
}
func (s *Service) credential(ctx context.Context, p l.Principal, id string) (fetcher.JSONCredential, error) {
	if id == "" {
		return fetcher.JSONCredential{}, nil
	}
	if s.credentials == nil {
		return fetcher.JSONCredential{}, d.ErrForbidden
	}
	return s.credentials.Resolve(ctx, p, id)
}
func digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func publicError(e error) error {
	if e == nil {
		return nil
	}
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, d.ErrInvalid, d.ErrForbidden, d.ErrNotFound, d.ErrConflict, d.ErrLimit, d.ErrDecode, d.ErrUnavailable} {
		if errors.Is(e, known) {
			return known
		}
	}
	switch {
	case errors.Is(e, fetcher.ErrAuthentication):
		return d.ErrForbidden
	case errors.Is(e, fetcher.ErrNotFound):
		return d.ErrNotFound
	case errors.Is(e, fetcher.ErrResponseTooLarge):
		return d.ErrLimit
	case errors.Is(e, fetcher.ErrUnsafeDestination), errors.Is(e, fetcher.ErrInvalidURL):
		return d.ErrInvalid
	}
	return d.ErrUpstream
}
func code(e error) string {
	e = publicError(e)
	if errors.Is(e, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if errors.Is(e, context.Canceled) {
		return "cancelled"
	}
	if e == nil {
		return ""
	}
	return e.Error()
}
func decodeJSON(raw []byte, value any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if e := dec.Decode(value); e != nil {
		return d.ErrDecode
	}
	if e := dec.Decode(new(any)); e != io.EOF {
		return d.ErrDecode
	}
	return nil
}
func (s *Service) fetchJSON(ctx context.Context, p l.Principal, endpoint, credential string, limit int64, value any) (*fetcher.Result, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	cred, e := s.credential(ctx, p, credential)
	if e != nil {
		return nil, e
	}
	out, e := s.fetch.FetchHuggingFace(ctx, endpoint, cred, limit)
	if e != nil {
		return nil, publicError(e)
	}
	if e = decodeJSON(out.Body, value); e != nil {
		return nil, e
	}
	return out, nil
}
func (s *Service) Search(ctx context.Context, p l.Principal, q, cursor string, limit int, credential string) (d.SearchResult, error) {
	result := d.SearchResult{Items: []d.SearchItem{}}
	if len(q) > 120 || len(cursor) > 8192 || limit < 1 || limit > 20 || len(credential) > 128 {
		return result, d.ErrInvalid
	}
	query := url.Values{"search": {q}, "limit": {fmt.Sprint(limit)}, "full": {"true"}}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	var records []struct {
		ID        string `json:"id"`
		SHA       string `json:"sha"`
		Downloads int64  `json:"downloads"`
		Likes     int64  `json:"likes"`
		Private   bool   `json:"private"`
		Gated     any    `json:"gated"`
	}
	out, e := s.fetchJSON(ctx, p, "https://huggingface.co/api/datasets?"+query.Encode(), credential, 2<<20, &records)
	if e != nil {
		return result, e
	}
	if len(records) > limit {
		return result, d.ErrLimit
	}
	for _, r := range records {
		if !d.ValidRepo(r.ID) {
			return result, d.ErrDecode
		}
		gated := r.Gated != nil && r.Gated != false && r.Gated != "false"
		result.Items = append(result.Items, d.SearchItem{RepoID: r.ID, SHA: r.SHA, Downloads: r.Downloads, Likes: r.Likes, Private: r.Private, Gated: gated})
	}
	for _, header := range out.Header.Values("Link") {
		for _, link := range strings.Split(header, ",") {
			if !strings.Contains(link, `rel="next"`) {
				continue
			}
			start := strings.Index(link, "<")
			end := strings.Index(link, ">")
			if start < 0 || end <= start {
				continue
			}
			u, e := url.Parse(link[start+1 : end])
			if e != nil || u.Scheme != "https" || u.Host != "huggingface.co" || u.Path != "/api/datasets" {
				return result, d.ErrDecode
			}
			result.NextCursor = u.Query().Get("cursor")
			if len(result.NextCursor) > 8192 {
				return result, d.ErrLimit
			}
		}
	}
	return result, nil
}
func fileURL(repo, revision, file string) string {
	parts := strings.Split(file, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return "https://huggingface.co/datasets/" + repo + "/resolve/" + revision + "/" + strings.Join(parts, "/")
}
func (s *Service) Preview(ctx context.Context, p l.Principal, in d.Request) (d.Preview, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return d.Preview{}, e
	}
	work, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	out, e := s.acquire(work, p, in)
	if e != nil {
		return d.Preview{}, publicError(e)
	}
	if len(out.Rows) > 20 {
		out.Rows = out.Rows[:20]
	}
	// Browsers parse JSON numbers as IEEE-754. Preview presents numeric tokens
	// as exact strings while the import path retains actual json.Number values.
	out.NumberEncoding = "exact_string"
	for i, row := range out.Rows {
		copy := append([]any(nil), row...)
		for j, value := range copy {
			if number, ok := value.(json.Number); ok {
				copy[j] = string(number)
			}
		}
		out.Rows[i] = copy
	}
	out.PreviewRows = len(out.Rows)
	return out, nil
}
func (s *Service) acquire(ctx context.Context, p l.Principal, in d.Request) (d.Preview, error) {
	out := d.Preview{}
	if s.decoder == nil {
		return out, d.ErrUnavailable
	}
	if e := in.Validate(); e != nil {
		return out, e
	}
	dataset, e := s.Dataset(ctx, p, in.RepoID, in.Revision, in.CredentialID)
	if e != nil {
		return out, e
	}
	allowed := map[string]bool{}
	for _, config := range dataset.Configs {
		if config.Name != in.Config {
			continue
		}
		for _, split := range config.Splits {
			if split.Name == in.Split {
				for _, f := range split.Files {
					allowed[f] = true
				}
			}
		}
	}
	files := map[string]d.File{}
	for _, f := range dataset.Files {
		files[f.Path] = f
	}
	for _, f := range in.Files {
		if !allowed[f] || !files[f].Supported {
			return out, d.ErrInvalid
		}
	}
	raw, _ := json.Marshal(in)
	out.Manifest = d.Manifest{RepoID: in.RepoID, Revision: dataset.Revision, Config: in.Config, Split: in.Split, Mapping: in.FieldMapping, License: dataset.License, Files: []d.FileReceipt{}, ExcludedColumns: []string{}, AcquiredAt: time.Now().UTC(), RequestHash: digest(raw)}
	out.ColumnTypes = map[string]string{}
	sources := map[string]string{}
	total := int64(0)
	totalKnown := true
	selected := map[string]bool{}
	for _, c := range in.FieldMapping.Columns {
		selected[c.Source] = true
	}
	selected[in.FieldMapping.TextField] = true
	selected[in.FieldMapping.IDField] = true
	for i, file := range in.Files {
		if len(out.Rows) >= in.MaxRows {
			out.Manifest.Truncated = true
			totalKnown = false
			break
		}
		if e = s.auth(ctx, p, "read"); e != nil {
			return out, e
		}
		cred, e := s.credential(ctx, p, in.CredentialID)
		if e != nil {
			return out, e
		}
		limit := min(d.MaxFileBytes, in.MaxBytes-out.Manifest.Bytes)
		if limit < 1 {
			return out, d.ErrLimit
		}
		if files[file].Size != nil && *files[file].Size > limit {
			return out, d.ErrLimit
		}
		data, e := s.fetch.FetchHuggingFace(ctx, fileURL(in.RepoID, in.Revision, file), cred, limit)
		if e != nil {
			return out, publicError(e)
		}
		hash := digest(data.Body)
		if expected := files[file].SHA256; expected != "" && expected != hash {
			return out, d.ErrConflict
		}
		decoded, e := s.decoder.Decode(ctx, data.Body, files[file].Format, file, in.MaxRows-len(out.Rows), in.FieldMapping)
		if e != nil {
			return out, publicError(e)
		}
		var rows [][]any
		if e = decodeJSON([]byte(decoded.RowsEncoded), &rows); e != nil {
			return out, e
		}
		if len(rows) == 0 || len(rows) > in.MaxRows-len(out.Rows) {
			return out, d.ErrDecode
		}
		if i == 0 {
			out.Columns = decoded.Columns
		} else {
			a, _ := json.Marshal(out.Columns)
			b, _ := json.Marshal(decoded.Columns)
			if !bytes.Equal(a, b) {
				return out, d.ErrDecode
			}
		}
		for key, value := range decoded.ColumnTypes {
			if prior, ok := out.ColumnTypes[key]; ok && prior != value {
				out.ColumnTypes[key] = "MIXED"
			} else {
				out.ColumnTypes[key] = value
			}
		}
		for _, c := range decoded.SourceColumns {
			if prior, ok := sources[c.Name]; ok && prior != c.Type {
				sources[c.Name] = "MIXED"
			} else {
				sources[c.Name] = c.Type
			}
		}
		out.Rows = append(out.Rows, rows...)
		out.Manifest.Bytes += int64(len(data.Body))
		out.Manifest.Files = append(out.Manifest.Files, d.FileReceipt{Path: file, SHA256: hash, Bytes: int64(len(data.Body)), Rows: len(rows), TotalRows: decoded.TotalRows, Truncated: decoded.Truncated})
		out.Manifest.Truncated = out.Manifest.Truncated || decoded.Truncated
		if decoded.TotalRows == nil {
			totalKnown = false
		} else {
			total += *decoded.TotalRows
		}
	}
	out.Manifest.Rows = len(out.Rows)
	out.Manifest.Sampled = out.Manifest.Truncated
	if totalKnown {
		out.Manifest.TotalRows = &total
	}
	for name, typ := range sources {
		out.SourceColumns = append(out.SourceColumns, d.Column{Name: name, Type: typ})
		if !selected[name] {
			out.Manifest.ExcludedColumns = append(out.Manifest.ExcludedColumns, name)
		}
	}
	sort.Slice(out.SourceColumns, func(i, j int) bool { return out.SourceColumns[i].Name < out.SourceColumns[j].Name })
	sort.Strings(out.Manifest.ExcludedColumns)
	input := l.Input{Kind: "table", Columns: out.Columns, ColumnTypes: out.ColumnTypes, Rows: out.Rows}
	if e = input.Validate(); e != nil {
		return out, d.ErrDecode
	}
	encoded, e := json.Marshal(input)
	if e != nil || len(encoded) > 4<<20 {
		return out, d.ErrLimit
	}
	if e = s.auth(ctx, p, "read"); e != nil {
		return out, e
	}
	if _, e = s.credential(ctx, p, in.CredentialID); e != nil {
		return out, e
	}
	return out, nil
}
func (s *Service) Import(ctx context.Context, p l.Principal, in d.Request, key string) (*d.Import, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	if s.decoder == nil {
		return nil, d.ErrUnavailable
	}
	if in.Validate() != nil || strings.TrimSpace(key) == "" || len(key) > 200 {
		return nil, d.ErrInvalid
	}
	if _, e := s.credential(ctx, p, in.CredentialID); e != nil {
		return nil, e
	}
	raw, _ := json.Marshal(in)
	receipt, created, e := s.repo.Reserve(ctx, p, digest([]byte(key)), digest(raw), in)
	if e != nil {
		return nil, e
	}
	if !created {
		return s.GetImport(ctx, p, receipt.ID)
	}
	work, cancel := context.WithDeadline(ctx, receipt.Deadline)
	defer cancel()
	result, e := s.acquire(work, p, in)
	var refs []l.RefID
	if e == nil {
		var payload []byte
		payload, e = json.Marshal(struct {
			l.Input
			Manifest d.Manifest `json:"huggingface_manifest"`
		}{l.Input{Kind: "table", Columns: result.Columns, ColumnTypes: result.ColumnTypes, Rows: result.Rows}, result.Manifest})
		if e == nil {
			var ref l.DataAssetRef
			ref, e = s.assets.ImportDerived(work, p, l.RawAsset, "huggingface-import:"+receipt.ID, nil, payload, l.PolicySnapshot{Sensitivity: "unknown", Scopes: []string{"read"}, Version: "1"})
			if e == nil {
				refs = []l.RefID{ref.RefID}
			}
		}
	}
	if e == nil {
		e = s.auth(work, p, "read")
	}
	if e == nil {
		_, e = s.credential(work, p, in.CredentialID)
	}
	final, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	var manifest *d.Manifest
	if result.Manifest.RepoID != "" {
		manifest = &result.Manifest
	}
	done, finishErr := s.repo.Finish(final, p, receipt.ID, manifest, refs, code(e))
	if finishErr != nil {
		return nil, finishErr
	}
	if e != nil {
		return done, publicError(e)
	}
	if done.Status != "succeeded" {
		return done, d.ErrConflict
	}
	return done, nil
}
func (s *Service) GetImport(ctx context.Context, p l.Principal, id string) (*d.Import, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	receipt, e := s.repo.Get(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if _, e = s.credential(ctx, p, receipt.Request.CredentialID); e != nil {
		return nil, e
	}
	if receipt.Status == "succeeded" {
		for _, ref := range receipt.OutputRefs {
			if _, _, e = s.assets.Resolve(ctx, p.TenantID, p.UserID, ref); e != nil {
				return nil, publicError(e)
			}
		}
	}
	return receipt, nil
}

// Called by lifecycle's origin gate. Never Resolve this same asset recursively.
func (s *Service) ValidateGovernedOrigin(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
	if e := s.auth(ctx, p, "read"); e != nil {
		return e
	}
	id := strings.TrimPrefix(ref.ProducerID, "huggingface-import:")
	if id == ref.ProducerID {
		return d.ErrForbidden
	}
	receipt, e := s.repo.Get(ctx, p, id)
	if e != nil {
		return d.ErrForbidden
	}
	if receipt.Status != "succeeded" || receipt.Manifest == nil || receipt.Manifest.RequestHash != receipt.RequestHash {
		return d.ErrForbidden
	}
	if _, e = s.credential(ctx, p, receipt.Request.CredentialID); e != nil {
		return e
	}
	for _, out := range receipt.OutputRefs {
		if out == ref.RefID && ref.TenantID == p.TenantID && ref.Kind == l.RawAsset {
			return nil
		}
	}
	return d.ErrForbidden
}
func Capabilities() any {
	return map[string]any{"id": "huggingface", "search": true, "fixed_revision_preview": true, "static_file_import": true, "formats": []string{"jsonl", "json", "csv", "jsonl.gz", "json.gz", "csv.gz", "parquet"}, "max_rows": 1000, "max_files": 4, "max_file_bytes": d.MaxFileBytes, "max_total_bytes": d.MaxDownloadBytes, "max_output_bytes": 4 << 20, "execution": "bounded synchronous; interrupted imports require a new explicit idempotency key", "dataset_scripts": false, "arbitrary_urls": false, "nested_values": "explicit json encoding only", "credential_origin": "https://huggingface.co/", "automatic_label_fields": false}
}
func (s *Service) Capabilities() any {
	capabilities := Capabilities().(map[string]any)
	capabilities["decoder_ready"] = s.decoder != nil
	capabilities["fixed_revision_preview"] = s.decoder != nil
	capabilities["static_file_import"] = s.decoder != nil
	return capabilities
}
