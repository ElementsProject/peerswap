package peerswaprpc

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPagingBounds_UnpagedIfEmpty(t *testing.T) {
	start, end, nextToken, err := pagingBounds(10, 0, "", true)
	require.NoError(t, err)
	require.Equal(t, 0, start)
	require.Equal(t, 10, end)
	require.Equal(t, "", nextToken)
}

func TestPagingBounds_DefaultPageSizeWhenTokenPresent(t *testing.T) {
	start, end, nextToken, err := pagingBounds(150, 0, "0", true)
	require.NoError(t, err)
	require.Equal(t, 0, start)
	require.Equal(t, defaultPageSize, end)
	require.Equal(t, "100", nextToken)
}

func TestPagingBounds_PageSizeAndToken(t *testing.T) {
	start, end, nextToken, err := pagingBounds(10, 3, "3", true)
	require.NoError(t, err)
	require.Equal(t, 3, start)
	require.Equal(t, 6, end)
	require.Equal(t, "6", nextToken)
}

func TestPagingBounds_LastPage(t *testing.T) {
	start, end, nextToken, err := pagingBounds(10, 3, "9", true)
	require.NoError(t, err)
	require.Equal(t, 9, start)
	require.Equal(t, 10, end)
	require.Equal(t, "", nextToken)
}

func TestPagingBounds_TokenBeyondEnd(t *testing.T) {
	start, end, nextToken, err := pagingBounds(10, 3, "20", true)
	require.NoError(t, err)
	require.Equal(t, 10, start)
	require.Equal(t, 10, end)
	require.Equal(t, "", nextToken)
}

func TestPagingBounds_InvalidToken(t *testing.T) {
	_, _, _, err := pagingBounds(10, 3, "not-a-number", true)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPagingBounds_MaxPageSizeClamp(t *testing.T) {
	start, end, nextToken, err := pagingBounds(5000, 50000, "0", true)
	require.NoError(t, err)
	require.Equal(t, 0, start)
	require.Equal(t, maxPageSize, end)
	require.Equal(t, "1000", nextToken)
}
