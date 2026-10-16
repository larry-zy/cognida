package hivewire

import (
	"database/sql/driver"
	h "github.com/beltran/gohive/v2/hiveserver"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHiveColumnarPagePreservesPrecisionAndNullBits(t *testing.T) {
	result, e := decodeColumns([]*h.TColumn{{StringVal: &h.TStringColumn{Values: []string{"00123", "9007199254740993.123456789"}, Nulls: []byte{0}}}, {I64Val: &h.TI64Column{Values: []int64{9223372036854775807, 0}, Nulls: []byte{2}}}}, 2)
	require.NoError(t, e)
	require.Equal(t, [][]driver.Value{{"00123", int64(9223372036854775807)}, {"9007199254740993.123456789", nil}}, result)
	_, e = decodeColumns([]*h.TColumn{{StringVal: &h.TStringColumn{Values: []string{"x"}}}, {I64Val: &h.TI64Column{Values: []int64{1, 2}}}}, 2)
	require.Error(t, e)
	_, e = decodeColumns([]*h.TColumn{{BinaryVal: &h.TBinaryColumn{Values: [][]byte{{1}}}}}, 1)
	require.Error(t, e)
}
