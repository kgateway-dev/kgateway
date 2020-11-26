package matchers

import (
	"github.com/golang/protobuf/proto"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
)

func MatchProto(msg proto.Message) types.GomegaMatcher {
	return &protoMatcherImpl{
		msg: msg,
	}
}

type protoMatcherImpl struct {
	msg proto.Message
}

func (p *protoMatcherImpl) Match(actual interface{}) (success bool, err error) {
	msg, ok := actual.(proto.Message)
	if !ok {
		return false, nil
	}
	return proto.Equal(msg, p.msg), nil
}

func (p *protoMatcherImpl) FailureMessage(actual interface{}) (message string) {
	msg, ok := actual.(proto.Message)
	if !ok {
		format.Message(actual, "To be identical to", p.msg.String())
	}
	return format.Message(msg.String(), "To be identical to", p.msg.String())
}

func (p *protoMatcherImpl) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "Not to be identical to", p.msg)
}
