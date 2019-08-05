package printers

import (
	"encoding/json"
	"fmt"

	"github.com/solo-io/go-utils/errors"
)

type OutputType int

const (
	KUBE_YAML OutputType = iota
	YAML
	JSON
	TABLE
)

var (
	_OutputTypeToValue = map[string]OutputType{
		"yaml":      YAML,
		"yml":       YAML,
		"kube-yaml": KUBE_YAML,
		"json":      JSON,
		"table":     TABLE,
	}

	_OutputValueToType = map[OutputType]string{
		YAML:      "yaml",
		KUBE_YAML: "kube-yaml",
		JSON:      "json",
		TABLE:     "table",
	}
)

func (o *OutputType) String() string {
	return _OutputValueToType[*o]
}

func (o *OutputType) Set(s string) error {
	val, ok := _OutputTypeToValue[s]
	if !ok {
		return errors.Errorf("%s is not a valid output type", s)
	}
	*o = val
	return nil
}

func (o *OutputType) Type() string {
	return "OutputType"
}

func (o OutputType) MarshalJSON() ([]byte, error) {
	if s, ok := interface{}(o).(fmt.Stringer); ok {
		return json.Marshal(s.String())
	}
	s, ok := _OutputValueToType[o]
	if !ok {
		return nil, errors.Errorf("invalid OutputType type: %d", o)
	}
	return json.Marshal(s)
}

func (o *OutputType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return errors.Errorf("OutputType should be a string, got %s", data)
	}
	v, ok := _OutputTypeToValue[s]
	if !ok {
		return errors.Errorf("invalid OutputType %q", s)
	}
	*o = v
	return nil
}
