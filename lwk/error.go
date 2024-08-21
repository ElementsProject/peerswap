package lwk

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// electrumRPCError represents the structure of an RPC error response
type electrumRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Regular expression to match RPC error messages with any prefix
var re = regexp.MustCompile(`^(.*) RPC error: (.*)$`)

// parseRPCError parses an error and extracts the RPC error code and message if present
func parseRPCError(err error) (*electrumRPCError, error) {
	var rpcErr electrumRPCError
	errStr := err.Error()

	matches := re.FindStringSubmatch(errStr)

	if len(matches) == 3 { // Prefix and JSON payload extracted successfully
		errJSON := matches[2]
		if jerr := json.Unmarshal([]byte(errJSON), &rpcErr); jerr != nil {
			return nil, fmt.Errorf("error parsing rpc error: %v", jerr)
		}
	} else {
		// If no RPC error pattern is found, return the original error
		return nil, errors.New(errStr)
	}

	return &rpcErr, nil
}
