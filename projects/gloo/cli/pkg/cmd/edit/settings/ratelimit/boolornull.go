package ratelimit

import "strconv"

// -- bool Value
type boolValue struct {
	Value **bool
}

func newBoolValue(val *bool, p **bool) *boolValue {
	*p = val
	return &boolValue{Value: p}
}

func (b *boolValue) Set(s string) error {
	v, err := strconv.ParseBool(s)
	*b.Value = &v
	return err
}

func (b *boolValue) Type() string {
	return "bool"
}

func (b *boolValue) String() string {
	if *b.Value == nil {
		return "nil"
	}
	return strconv.FormatBool(**b.Value)
}

func (b *boolValue) IsBoolFlag() bool { return true }
