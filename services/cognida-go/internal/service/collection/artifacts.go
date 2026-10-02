package collection

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	domain "cognida/internal/model/collection"
	"cognida/internal/model/objectstore"
)

var errArtifact = errors.New("collection artifact unavailable")

type rawSnapshot struct {
	Version      int    `json:"version"`
	CanonicalURL string `json:"canonical_url"`
	ContentType  string `json:"content_type"`
	StatusCode   int    `json:"status_code"`
	Hash         string `json:"hash"`
	Body         []byte `json:"body"`
}

func artifactKey(item *domain.Item, kind string) string {
	identity := sha256.Sum256([]byte(item.TaskID + "\x00" + item.RunID + "\x00" + item.ID))
	return fmt.Sprintf("collection/%d/%x/%s", item.TenantID, identity, kind)
}
func (s *Service) persistRaw(ctx context.Context, item *domain.Item, response *domain.FetchResponse) error {
	if s.artifacts == nil {
		return nil
	}
	sum := sha256.Sum256(response.Body)
	hash := hex.EncodeToString(sum[:])
	key := artifactKey(item, "raw")
	if item.RawObjectKey == "" {
		envelope, err := json.Marshal(rawSnapshot{1, response.CanonicalURL, response.ContentType, response.StatusCode, hash, response.Body})
		if err != nil {
			return err
		}
		if _, err := s.artifacts.Put(ctx, objectstore.PutRequest{Key: key, Body: bytes.NewReader(envelope), Size: int64(len(envelope)), ContentType: "application/json"}); err != nil {
			return fmt.Errorf("%w: persist raw: %w", errArtifact, err)
		}
	}
	item.RawObjectKey, item.RawHash, item.RawSize = key, hash, int64(len(response.Body))
	return s.repository.UpdateItem(ctx, item)
}
func (s *Service) persistNormalized(ctx context.Context, item *domain.Item, content string) error {
	if s.artifacts == nil {
		return nil
	}
	key := artifactKey(item, "normalized")
	if _, err := s.artifacts.Put(ctx, objectstore.PutRequest{Key: key, Body: strings.NewReader(content), Size: int64(len(content)), ContentType: "text/plain; charset=utf-8"}); err != nil {
		return fmt.Errorf("%w: persist normalized: %w", errArtifact, err)
	}
	item.NormalizedObjectKey, item.NormalizedSize = key, int64(len(content))
	return s.repository.UpdateItem(ctx, item)
}
func (s *Service) fetchOrRestore(ctx context.Context, task *domain.Task, item *domain.Item, source *domain.Source) (*domain.FetchResponse, error) {
	if s.artifacts == nil {
		return s.fetcher.Fetch(ctx, domain.FetchRequest{URL: source.URL, MaxResponseBytes: task.Limits.MaxResponseBytes})
	}
	key := item.RawObjectKey
	if key == "" {
		key = artifactKey(item, "raw")
	}
	reader, _, err := s.artifacts.Get(ctx, key)
	if errors.Is(err, objectstore.ErrNotFound) && item.RawObjectKey == "" {
		return s.fetcher.Fetch(ctx, domain.FetchRequest{URL: source.URL, MaxResponseBytes: task.Limits.MaxResponseBytes})
	}
	if err != nil {
		return nil, fmt.Errorf("%w: restore raw: %w", errArtifact, err)
	}
	defer reader.Close()
	limit := task.Limits.MaxResponseBytes
	if limit <= 0 {
		limit = 2 << 20
	}
	// Base64 response envelope adds 4/3 payload plus bounded HTTP metadata.
	envelopeLimit := limit*4/3 + 65536
	raw, err := io.ReadAll(io.LimitReader(reader, envelopeLimit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errArtifact, err)
	}
	var snapshot rawSnapshot
	if int64(len(raw)) > envelopeLimit || json.Unmarshal(raw, &snapshot) != nil || snapshot.Version != 1 {
		return nil, fmt.Errorf("%w: invalid raw snapshot envelope", errArtifact)
	}
	sum := sha256.Sum256(snapshot.Body)
	hash := hex.EncodeToString(sum[:])
	if int64(len(snapshot.Body)) > limit || hash != snapshot.Hash || (item.RawHash != "" && (item.RawHash != hash || item.RawSize != int64(len(snapshot.Body)))) {
		return nil, fmt.Errorf("%w: raw integrity check failed", errArtifact)
	}
	item.RawObjectKey, item.RawHash, item.RawSize = key, hash, int64(len(snapshot.Body))
	return &domain.FetchResponse{CanonicalURL: snapshot.CanonicalURL, ContentType: snapshot.ContentType, StatusCode: snapshot.StatusCode, Body: snapshot.Body}, nil
}
