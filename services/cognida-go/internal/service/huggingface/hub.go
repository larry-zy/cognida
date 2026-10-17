package huggingface

import (
	d "cognida/internal/model/huggingface"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var hubFileSHA256 = regexp.MustCompile(`^[a-f0-9]{64}$`)

type hubInfo struct {
	ID       string `json:"id"`
	SHA      string `json:"sha"`
	CardData struct {
		License any `json:"license"`
		Configs []struct {
			Name      string          `json:"config_name"`
			DataFiles json.RawMessage `json:"data_files"`
		} `json:"configs"`
	} `json:"cardData"`
	Siblings []struct {
		Path string `json:"rfilename"`
		Size *int64 `json:"size"`
		LFS  *struct {
			SHA  string `json:"sha256"`
			Size int64  `json:"size"`
		} `json:"lfs"`
	} `json:"siblings"`
}

func (s *Service) Dataset(ctx context.Context, p l.Principal, repo, revision, credential string) (d.Dataset, error) {
	out := d.Dataset{Files: []d.File{}, Configs: []d.Config{}, Warnings: []string{}}
	if !d.ValidRepo(repo) || (revision != "main" && !d.ValidCommit(revision)) || len(credential) > 128 {
		return out, d.ErrInvalid
	}
	var info hubInfo
	_, e := s.fetchJSON(ctx, p, "https://huggingface.co/api/datasets/"+repo+"/revision/"+url.PathEscape(revision)+"?blobs=true", credential, 2<<20, &info)
	if e != nil {
		return out, e
	}
	if info.ID != repo || !d.ValidCommit(info.SHA) || (revision != "main" && info.SHA != revision) {
		return out, d.ErrConflict
	}
	if len(info.Siblings) > 4000 || len(info.CardData.Configs) > 500 {
		return out, d.ErrLimit
	}
	out.RepoID = repo
	out.Revision = info.SHA
	out.License = "unknown"
	if text, ok := info.CardData.License.(string); ok && len(text) <= 512 {
		out.License = text
	} else if value, ok := info.CardData.License.([]any); ok {
		raw, _ := json.Marshal(value)
		if len(raw) <= 512 {
			out.License = string(raw)
		}
	}
	seen := map[string]bool{}
	for _, file := range info.Siblings {
		if !d.ValidPath(file.Path) || seen[file.Path] {
			return out, d.ErrDecode
		}
		seen[file.Path] = true
		f := d.File{Path: file.Path, Size: file.Size, Format: d.Format(file.Path)}
		f.Supported = f.Format != ""
		if file.LFS != nil {
			f.Size = &file.LFS.Size
			if !hubFileSHA256.MatchString(file.LFS.SHA) {
				return out, d.ErrDecode
			}
			f.SHA256 = file.LFS.SHA
		}
		if f.Size != nil && *f.Size < 0 {
			return out, d.ErrDecode
		}
		out.Files = append(out.Files, f)
	}
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].Path < out.Files[j].Path })
	if seen["README.md"] {
		cred, e := s.credential(ctx, p, credential)
		if e != nil {
			return out, e
		}
		card, e := s.fetch.FetchHuggingFace(ctx, fileURL(repo, info.SHA, "README.md"), cred, 256<<10)
		if e != nil {
			return out, publicError(e)
		}
		if !utf8.Valid(card.Body) {
			return out, d.ErrDecode
		}
		out.Card = string(card.Body)
	} else {
		out.Warnings = append(out.Warnings, "No README.md exists at this commit; license is metadata only.")
	}
	for _, config := range info.CardData.Configs {
		if config.Name == "" || len(config.Name) > 128 {
			return out, d.ErrDecode
		}
		var mappings []struct {
			Split string          `json:"split"`
			Path  json.RawMessage `json:"path"`
		}
		if json.Unmarshal(config.DataFiles, &mappings) != nil {
			out.Warnings = append(out.Warnings, "Unsupported config data_files shape: "+config.Name)
			continue
		}
		c := d.Config{Name: config.Name, Splits: []d.Split{}}
		groups := map[string]map[string]bool{}
		for _, mapping := range mappings {
			if mapping.Split == "" || len(mapping.Split) > 128 {
				return out, d.ErrDecode
			}
			var patterns []string
			var one string
			if json.Unmarshal(mapping.Path, &one) == nil {
				patterns = []string{one}
			} else if json.Unmarshal(mapping.Path, &patterns) != nil {
				return out, d.ErrDecode
			}
			if len(patterns) > 100 {
				return out, d.ErrLimit
			}
			if groups[mapping.Split] == nil {
				groups[mapping.Split] = map[string]bool{}
			}
			for _, pattern := range patterns {
				if len(pattern) > 512 || strings.ContainsAny(pattern, "\\\x00\r\n") || strings.HasPrefix(pattern, "/") || strings.Contains(pattern, "..") {
					return out, d.ErrDecode
				}
				for _, file := range out.Files {
					if file.Supported && matchPath(pattern, file.Path) {
						groups[mapping.Split][file.Path] = true
					}
				}
			}
		}
		for name, files := range groups {
			split := d.Split{Name: name, Files: []string{}}
			for file := range files {
				split.Files = append(split.Files, file)
			}
			sort.Strings(split.Files)
			if len(split.Files) > 0 {
				c.Splits = append(c.Splits, split)
			}
		}
		sort.Slice(c.Splits, func(i, j int) bool { return c.Splits[i].Name < c.Splits[j].Name })
		if len(c.Splits) > 0 {
			out.Configs = append(out.Configs, c)
		}
	}
	if len(out.Configs) == 0 {
		split := d.Split{Name: "selected", Files: []string{}}
		for _, file := range out.Files {
			if file.Supported {
				split.Files = append(split.Files, file.Path)
			}
		}
		out.Configs = []d.Config{{Name: "static_files", Splits: []d.Split{split}}}
		out.Warnings = append(out.Warnings, "No supported config/split manifest. static_files/selected is explicit file selection, not a publisher-declared split.")
	}
	sort.Slice(out.Configs, func(i, j int) bool { return out.Configs[i].Name < out.Configs[j].Name })
	return out, nil
}
func matchPath(pattern, file string) bool {
	// Hub data_files are path globs, never URLs or executable loaders. Implement
	// ** as zero or more path segments; all other glob syntax uses path.Match.
	parts := strings.Split(pattern, "/")
	files := strings.Split(file, "/")
	seen := map[[2]int]bool{}
	var match func(int, int) bool
	match = func(i, j int) bool {
		state := [2]int{i, j}
		if seen[state] {
			return false
		}
		seen[state] = true
		if i == len(parts) {
			return j == len(files)
		}
		if parts[i] == "**" {
			for k := j; k <= len(files); k++ {
				if match(i+1, k) {
					return true
				}
			}
			return false
		}
		if j >= len(files) {
			return false
		}
		ok, e := path.Match(parts[i], files[j])
		return e == nil && ok && match(i+1, j+1)
	}
	return match(0, 0)
}
