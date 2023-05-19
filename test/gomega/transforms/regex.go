package transforms

import (
	"fmt"
	"regexp"
	"strconv"
)

// IntRegexTransform returns a function that can be used to transform a byte array (e.g., an http.Response body) into an int
// by applying a regular expression to the byte array. The regular expression must have exactly one capture group.
// The function will return the integer value of the capture group.
func IntRegexTransform(regexp *regexp.Regexp) func(body []byte) (int, error) {
	return func(body []byte) (int, error) {
		matches := regexp.FindAllStringSubmatch(string(body), -1)
		if len(matches) != 1 {
			return 0, fmt.Errorf("found %d matches, expected 1", len(matches))
		}

		// matches[0] is the first match
		// matches[0][1] is the first capture group
		matchCount, conversionErr := strconv.Atoi(matches[0][1])
		if conversionErr != nil {
			return 0, conversionErr
		}

		return matchCount, nil
	}
}
