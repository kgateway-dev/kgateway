package transforms

import (
	"fmt"
	"regexp"
	"strconv"
)

// IntRegexTransform parses a http.Response body
// and returns the integer value of the provided regular expression
func IntRegexTransform(regexp *regexp.Regexp) func(body []byte) (int, error) {
	return func(body []byte) (int, error) {
		matches := regexp.FindAllStringSubmatch(string(body), -1)
		if len(matches) != 1 {
			return 0, fmt.Errorf("found %d matches, expected 1", len(matches))
		}

		matchCount, conversionErr := strconv.Atoi(matches[0][1])
		if conversionErr != nil {
			return 0, conversionErr
		}

		return matchCount, nil
	}
}
