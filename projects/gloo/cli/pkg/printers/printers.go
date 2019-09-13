package printers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"

	"github.com/solo-io/go-utils/errors"
)

type OutputType int

const (
	TABLE OutputType = iota
	YAML
	JSON
	KUBE_YAML
	WIDE
)

const DryRunFallbackOutputType = KUBE_YAML

type outputTypeProperties struct {
	outputType OutputType
	// the first entry will be the default
	names []string
	// if the type is a table output, it does not support dry run
	isTable bool
}

var typeProperties = []outputTypeProperties{
	{TABLE, []string{"table"}, true},
	{YAML, []string{"yaml", "yml"}, false},
	{KUBE_YAML, []string{"kube-yaml"}, false},
	{JSON, []string{"json"}, false},
	{WIDE, []string{"wide"}, true},
}

var (
	_OutputTypeToValue = map[string]OutputType{}
	// "yaml":      YAML,
	// "yml":       YAML,

	_OutputValueToType = map[OutputType]string{}
	// YAML:      "yaml",

	_OutputValueToIsTable = map[OutputType]bool{}
	// YAML:      false,
)

func init() {
	for _, tp := range typeProperties {
		if len(tp.names) == 0 {
			// this should not happen, check just in case new types are added incorrectly
			contextutils.LoggerFrom(context.TODO()).Fatalw("initialization of invalid output type",
				zap.Any("outputType", tp.outputType))
		}
		for nameIndex, name := range tp.names {
			if nameIndex == 0 {
				_OutputTypeToValue[name] = tp.outputType
			}
			_OutputValueToType[tp.outputType] = name
		}
		_OutputValueToIsTable[tp.outputType] = tp.isTable
	}
}

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

func (o *OutputType) IsTable() bool {
	return _OutputValueToIsTable[*o]
}
