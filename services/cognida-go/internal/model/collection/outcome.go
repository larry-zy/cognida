package collection

// OutcomeSummary separates completed work from successful ingestion. Duplicate
// items with a knowledge reference are usable results, not collection failures.
// It is derived from durable items and does not introduce a second stored state.
type OutcomeSummary struct {
	Outcome  string `json:"outcome"`
	Total    int    `json:"total"`
	Accepted int    `json:"accepted"`
	Skipped  int    `json:"skipped"`
	Rejected int    `json:"rejected"`
	Failed   int    `json:"failed"`
	Pending  int    `json:"pending"`
	Usable   int    `json:"usable"`
}

func SummarizeItems(expected int, items []*Item) OutcomeSummary {
	s := OutcomeSummary{Total: max(expected, len(items))}
	for _, item := range items {
		if item == nil {
			continue
		}
		switch item.Status {
		case ItemStatusAccepted, ItemStatusSkipped:
			if item.KnowledgeID == "" {
				s.Failed++ // An incomplete receipt must never count as usable.
			} else if item.Status == ItemStatusAccepted {
				s.Accepted++
			} else {
				s.Skipped++
			}
		case ItemStatusRejected:
			s.Rejected++
		case ItemStatusFailed:
			s.Failed++
		}
	}
	s.Usable = s.Accepted + s.Skipped
	s.Pending = s.Total - s.Usable - s.Rejected - s.Failed
	switch {
	case s.Pending > 0:
		s.Outcome = "running"
	case s.Usable == 0:
		s.Outcome = "failed"
	case s.Failed+s.Rejected > 0:
		s.Outcome = "partial"
	default:
		s.Outcome = "success"
	}
	return s
}
