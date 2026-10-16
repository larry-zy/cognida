package hivewire

import (
	"context"
	"database/sql/driver"
	h "github.com/beltran/gohive/v2/hiveserver"
	"github.com/stretchr/testify/require"
	"testing"
)

type shortPages struct {
	cursor, calls int
	stalled       bool
	badOffset     bool
}

func (s *shortPages) FetchResults(_ context.Context, req *h.TFetchResultsReq) (*h.TFetchResultsResp, error) {
	if req.Orientation == h.TFetchOrientation_FETCH_FIRST {
		s.cursor = 0
	}
	s.calls++
	start := s.cursor
	values := []int64{}
	if !s.stalled && s.cursor < 5 {
		values = []int64{int64(s.cursor)}
		s.cursor++
	}
	if s.badOffset {
		start++
	}
	more := s.cursor < 5
	return &h.TFetchResultsResp{Status: &h.TStatus{StatusCode: h.TStatusCode_SUCCESS_STATUS}, HasMoreRows: &more, Results: &h.TRowSet{StartRowOffset: int64(start), Columns: []*h.TColumn{{I64Val: &h.TI64Column{Values: values}}}}}, nil
}

func TestFetchPrefixFollowsServerShortPagesAndRetriesFromBeginning(t *testing.T) {
	s := &shortPages{}
	values, err := fetchPrefix(context.Background(), s, &h.TOperationHandle{}, 1, 4)
	require.NoError(t, err)
	require.Equal(t, [][]driver.Value{{int64(0)}, {int64(1)}, {int64(2)}, {int64(3)}}, values)
	require.Equal(t, 4, s.calls)
	retry, err := fetchPrefix(context.Background(), s, &h.TOperationHandle{}, 1, 4)
	require.NoError(t, err)
	require.Equal(t, values, retry)
	last, err := fetchPrefix(context.Background(), s, &h.TOperationHandle{}, 1, 8)
	require.NoError(t, err)
	require.Len(t, last, 5)
}

func TestFetchPrefixRejectsStalledOrMisalignedPages(t *testing.T) {
	for _, s := range []*shortPages{{stalled: true}, {badOffset: true}} {
		_, err := fetchPrefix(context.Background(), s, &h.TOperationHandle{}, 1, 4)
		require.Error(t, err)
	}
}
