package admin_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/admin"
)

var _ = Describe("SnapshotResponseData", func() {

	DescribeTable("MarshalJSONString",
		func(response admin.SnapshotResponseData, expectedString string) {
			responseStr := response.MarshalJSONString()
			Expect(responseStr).To(MatchJSON(expectedString))
		},
		Entry("successful response can be formatted as json",
			admin.SnapshotResponseData{
				Data:  "my data",
				Error: nil,
			},
			"{\"data\":\"my data\",\"error\":\"\"}"),
		Entry("errored response can be formatted as json",
			admin.SnapshotResponseData{
				Data:  "",
				Error: errors.New("one error"),
			},
			"{\"data\":\"\",\"error\":\"one error\"}"),
		Entry("CR list can be formatted as json",
			admin.SnapshotResponseData{
				Data: []corev1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "name",
							Namespace: "namespace",
							ManagedFields: []metav1.ManagedFieldsEntry{{
								Manager: "manager",
							}},
						},
						TypeMeta: metav1.TypeMeta{
							Kind:       "kind",
							APIVersion: "version",
						},
					},
				},
				Error: nil,
			},
			"{\"data\":[{\"kind\":\"kind\",\"apiVersion\":\"version\",\"metadata\":{\"name\":\"name\",\"namespace\":\"namespace\",\"creationTimestamp\":null,\"managedFields\":[{\"manager\":\"manager\"}]},\"status\":{},\"spec\":{}}],\"error\":\"\"}"),
	)
})
