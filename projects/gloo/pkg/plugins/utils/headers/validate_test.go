package headers_test

import (
	"fmt"
	"unicode"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/headers"
)

var _ = Describe("Validate header keys", func() {

	generateInvalidEntries := func(character rune) []TableEntry {
		var entries []TableEntry
		entries = append(entries, Entry(fmt.Sprintf("just '%s'", string(character)), fmt.Sprintf("%s", string(character)), true))
		entries = append(entries, Entry(fmt.Sprintf("contains leading '%s'", string(character)), fmt.Sprintf("%sreserved", string(character)), true))
		entries = append(entries, Entry(fmt.Sprintf("contains trailing '%s'", string(character)), fmt.Sprintf("reserved%s", string(character)), true))
		entries = append(entries, Entry(fmt.Sprintf("contains '%s'", string(character)), fmt.Sprintf("rese%srved", string(character)), true))
		return entries
	}

	DescribeTable("Validates header keys", func(key string, errored bool) {
		Expect(headers.ValidateHeaderKey(key) != nil).To(Equal(errored))
	},
		generateInvalidEntries(':'),
		generateInvalidEntries('"'),
		generateInvalidEntries(' '),
		generateInvalidEntries('('),
		generateInvalidEntries(')'),
		generateInvalidEntries(','),
		generateInvalidEntries('/'),
		generateInvalidEntries(':'),
		generateInvalidEntries(';'),
		generateInvalidEntries('<'),
		generateInvalidEntries('='),
		generateInvalidEntries('>'),
		generateInvalidEntries('?'),
		generateInvalidEntries('@'),
		generateInvalidEntries('['),
		generateInvalidEntries('\\'),
		generateInvalidEntries(']'),
		generateInvalidEntries('{'),
		generateInvalidEntries('}'),
		generateInvalidEntries('>'),
		generateInvalidEntries(unicode.MaxASCII),
		Entry("valid header", "valid-header", false),
	)
})
