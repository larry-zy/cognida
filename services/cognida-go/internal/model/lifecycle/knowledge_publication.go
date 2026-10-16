package lifecycle

// KnowledgePublication contains only verified server-owned publication receipt
// fields. Current document/chunk integrity and label review still require the
// caller's atomic repository check before applying labels.
type KnowledgePublication struct {
	KnowledgeID   string
	ContentDigest string
	ChunkDigest   string
}
