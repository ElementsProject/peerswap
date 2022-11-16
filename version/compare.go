package version

import (
	"fmt"
	"regexp"
	"strconv"
)

// CompareVersionStrings compares the numeric part of a version string. Possible
// versions could be for example: v0.1.2, v22.11rc1.
// Returns true if `a` is higher or equal to `b` and false if not.
func CompareVersionStrings(a, b string) (bool, error) {
	re := regexp.MustCompile(`[0-9]+`)
	partsA := re.FindAllString(a, -1)
	partsB := re.FindAllString(b, -1)

	// Equalize lengths of version string slices, fill with 0.
	d := len(partsA) - len(partsB)
	if d > 0 {
		for i := 0; i < d; i++ {
			partsB = append(partsB, "0")
		}
	} else if d < 0 {
		for i := 0; i < -1*d; i++ {
			partsA = append(partsA, "0")
		}
	}

	// Convert strings to integers.
	var numericA []int
	var numericB []int
	for i := range partsA {
		n, err := strconv.Atoi(partsA[i])
		if err != nil {
			return false, fmt.Errorf("malformed version string %s: %w", version, err)
		}
		numericA = append(numericA, n)

		n, err = strconv.Atoi(partsB[i])
		if err != nil {
			return false, fmt.Errorf("malformed version string %s: %w", version, err)
		}
		numericB = append(numericB, n)
	}

	// Compare entries.
	for i := range numericA {
		if numericB[i] > numericA[i] {
			return false, nil
		}
		if numericA[i] > numericB[i] {
			return true, nil
		}
	}

	// Versions are the same
	return true, nil
}
