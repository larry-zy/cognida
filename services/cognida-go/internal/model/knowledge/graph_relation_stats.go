package knowledge

import "context"

// GraphRelationStatsRepository aggregates the complete scoped graph without the
// visualization repository's node/edge loading limits.
type GraphRelationStatsRepository interface {
	GetRelationStats(context.Context, NameSpace) (*RelationStatistics, error)
}
