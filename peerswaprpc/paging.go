package peerswaprpc

import (
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultPageSize = 100
	maxPageSize     = 1000
)

func pagingBounds(total int, pageSize uint32, pageToken string, unpagedIfEmpty bool) (start, end int, nextPageToken string, err error) {
	if total < 0 {
		return 0, 0, "", status.Error(codes.Internal, "invalid total size")
	}

	if unpagedIfEmpty && pageSize == 0 && pageToken == "" {
		return 0, total, "", nil
	}

	if pageToken != "" {
		offset, err := strconv.Atoi(pageToken)
		if err != nil || offset < 0 {
			return 0, 0, "", status.Error(codes.InvalidArgument, "invalid page_token")
		}
		start = offset
	}

	size := int(pageSize)
	if size <= 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}

	if start > total {
		start = total
	}

	end = start + size
	if end > total {
		end = total
	}

	if end < total {
		nextPageToken = strconv.Itoa(end)
	}

	return start, end, nextPageToken, nil
}
