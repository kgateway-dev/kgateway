package utils_test

import (
	"github.com/golang/protobuf/proto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/utils"

	gogostructpb "github.com/gogo/protobuf/types"
	structpb "github.com/golang/protobuf/ptypes/struct"
)

var _ = Describe("Any", func() {

	It("should convert golang message to any", func() {
		msg := &structpb.Struct{
			Fields: map[string]*structpb.Value{"test": {
				Kind: &structpb.Value_StringValue{StringValue: "foo"},
			}},
		}
		anymsg, err := MessageToAny(msg)
		Expect(err).NotTo(HaveOccurred())

		msg2, err := AnyToMessage(anymsg)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg2.(*structpb.Struct).Fields["test"].GetStringValue()).To(Equal(msg.Fields["test"].GetStringValue()))
	})

	It("should convert gogo message to any", func() {
		msg := &gogostructpb.Struct{
			Fields: map[string]*gogostructpb.Value{"test": {
				Kind: &gogostructpb.Value_StringValue{StringValue: "foo"},
			}},
		}
		anymsg, err := MessageToAny(msg)
		Expect(err).NotTo(HaveOccurred())

		msg2, err := AnyToMessage(anymsg)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg2.(*structpb.Struct).Fields["test"].GetStringValue()).To(Equal(msg.Fields["test"].GetStringValue()))
	})

	It("fails with nil message", func() {
		var msg proto.Message
		_, err := MessageToAny(msg)
		Expect(err).To(MatchError("MessageToAny: message cannot be nil"))
	})
})
