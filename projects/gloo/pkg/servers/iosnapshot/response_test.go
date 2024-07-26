package iosnapshot

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
)

var _ = Describe("SnapshotResponseData", func() {

	DescribeTable("MarshalJSON",
		func(response SnapshotResponseData, expectedString string) {
			bytes, err := response.MarshalJSON()
			Expect(err).NotTo(HaveOccurred())
			Expect(bytes).To(
				WithTransform(func(b []byte) string {
					return string(b)
				}, Equal(expectedString)),
			)
		},
		Entry("successful response can be formatted as json",
			SnapshotResponseData{
				Data:  "my data",
				Error: nil,
			},
			"{\"data\":\"my data\",\"error\":\"\"}"),
		Entry("errored response can be formatted as json",
			SnapshotResponseData{
				Data:  "",
				Error: eris.New("one error"),
			},
			"{\"data\":\"\",\"error\":\"one error\"}"),
	)
})
