package knowledge

import "encoding/json"

func mergeChunkConfig(existing string, strategy *string, size, overlap *int) (string, error) {
	config := map[string]any{"strategy": "paragraph", "chunk_size": 500, "chunk_overlap": 50}
	if existing != "" {
		if err := json.Unmarshal([]byte(existing), &config); err != nil {
			return "", &ValidationError{Field: "chunking_config", Message: "stored configuration is invalid"}
		}
	}
	if config == nil {
		config = map[string]any{"strategy": "paragraph", "chunk_size": 500, "chunk_overlap": 50}
	}
	if strategy != nil {
		switch *strategy {
		case "auto", "paragraph", "sentence", "fixed_size", "semantic", "recursive":
		default:
			return "", &ValidationError{Field: "chunk_strategy", Message: "unsupported strategy"}
		}
		config["strategy"] = *strategy
		if *strategy == "auto" {
			if size == nil {
				config["chunk_size"] = 0
			}
			if overlap == nil {
				config["chunk_overlap"] = 0
			}
		}
	}
	if size != nil {
		if *size < 0 || *size > 20000 {
			return "", &ValidationError{Field: "chunk_size", Message: "must be 0-20000"}
		}
		config["chunk_size"] = *size
	}
	if overlap != nil {
		if *overlap < 0 || *overlap > 10000 {
			return "", &ValidationError{Field: "chunk_overlap", Message: "must be 0-10000"}
		}
		config["chunk_overlap"] = *overlap
	}
	// Normalize through JSON so historical and newly patched numbers share a type.
	body, _ := json.Marshal(config)
	_ = json.Unmarshal(body, &config)
	n, _ := config["chunk_size"].(float64)
	o, _ := config["chunk_overlap"].(float64)
	if config["strategy"] != "auto" && n == 0 {
		n = 500
		config["chunk_size"] = n
		body, _ = json.Marshal(config)
	}
	if n > 0 && o >= n {
		return "", &ValidationError{Field: "chunk_overlap", Message: "must be less than chunk_size"}
	}
	return string(body), nil
}
