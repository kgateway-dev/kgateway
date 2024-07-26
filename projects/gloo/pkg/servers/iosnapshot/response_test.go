package iosnapshot

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
)

var _ = Describe("SnapshotResponseData", func() {

	DescribeTable("MarshalJSONString",
		func(response SnapshotResponseData, expectedString string) {
			responseStr := response.MarshalJSONString()
			Expect(responseStr).To(Equal(expectedString))
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
