package gogoutils

import (
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
)

func UInt32ProtoToGogo(pr *wrappers.UInt32Value) *types.UInt32Value {
	var ret *types.UInt32Value
	if pr != nil {
		ret = &types.UInt32Value{
			Value: pr.GetValue(),
		}
	}
	return ret
}

func UInt32GogoToProto(pr *types.UInt32Value) *wrappers.UInt32Value {
	var ret *wrappers.UInt32Value
	if pr != nil {
		ret = &wrappers.UInt32Value{
			Value: pr.GetValue(),
		}
	}
	return ret
}

func UInt64ProtoToGogo(pr *wrappers.UInt64Value) *types.UInt64Value {
	var ret *types.UInt64Value
	if pr != nil {
		ret = &types.UInt64Value{
			Value: pr.GetValue(),
		}
	}
	return ret
}

func UInt64GogoToProto(pr *types.UInt64Value) *wrappers.UInt64Value {
	var ret *wrappers.UInt64Value
	if pr != nil {
		ret = &wrappers.UInt64Value{
			Value: pr.GetValue(),
		}
	}
	return ret
}

func BoolProtoToGogo(pr *wrappers.BoolValue) *types.BoolValue {
	var ret *types.BoolValue
	if pr != nil {
		ret = &types.BoolValue{
			Value: pr.GetValue(),
		}
	}
	return ret
}

func BoolGogoToProto(pr *types.BoolValue) *wrappers.BoolValue {
	var ret *wrappers.BoolValue
	if pr != nil {
		ret = &wrappers.BoolValue{
			Value: pr.GetValue(),
		}
	}
	return ret
}

func DurationProtoToGogo(pr *duration.Duration) *types.Duration {
	var ret *types.Duration
	if pr != nil {
		ret = &types.Duration{
			Seconds: pr.GetSeconds(),
			Nanos:   pr.GetNanos(),
		}
	}
	return ret
}

func DurationGogoToProto(pr *types.Duration) *duration.Duration {
	var ret *duration.Duration
	if pr != nil {
		ret = &duration.Duration{
			Seconds: pr.GetSeconds(),
			Nanos:   pr.GetNanos(),
		}
	}
	return ret
}

func DurationStdToProto(pr *time.Duration) *duration.Duration {
	var ret *duration.Duration
	if pr != nil {
		ret = ptypes.DurationProto(*pr)
	}
	return ret
}

func DurationProtoToStd(pr *duration.Duration) *time.Duration {
	dur, _ := ptypes.Duration(pr)
	return &dur
}
