package azure

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/protoutil"

	multierror "github.com/hashicorp/go-multierror"
)

func EncodeUpstreamSpec(spec UpstreamSpec) *types.Struct {
	v1Spec, err := protoutil.MarshalStruct(spec)
	if err != nil {
		panic(err)
	}
	return v1Spec
}

func DecodeUpstreamSpec(generic v1.UpstreamSpec) (*UpstreamSpec, error) {
	s := new(UpstreamSpec)
	if err := protoutil.UnmarshalStruct(generic, s); err != nil {
		return nil, err
	}
	return s, s.Validate()
}

func (s *UpstreamSpec) Validate() error {
	if !isValidFunctionAppName(s.FunctionAppName) {
		return errors.New("function app name must be non-empty and can contain letters, digits and dashes")
	}

	return nil
}

func (s *UpstreamSpec) GetHostname() string {
	return fmt.Sprintf("%s.azurewebsites.net", s.FunctionAppName)
}

func EncodeFunctionSpec(spec FunctionSpec) *types.Struct {
	v1Spec, err := protoutil.MarshalStruct(spec)
	if err != nil {
		panic(err)
	}
	return v1Spec
}

func DecodeFunctionSpec(generic v1.FunctionSpec) (*FunctionSpec, error) {
	s := new(FunctionSpec)
	if err := protoutil.UnmarshalStruct(generic, s); err != nil {
		return nil, err
	}
	return s, s.Validate()
}

func (s *FunctionSpec) Validate() error {
	var result error

	if !isValidFunctionName(s.FunctionName) {
		result = multierror.Append(
			result,
			errors.New("function name must start with a letter and can contain letters, digits, dashes and underscores"))
	}
	if !isValidAuthLevel(s.AuthLevel) {
		result = multierror.Append(
			result,
			errors.New("authentication level must be one of \"anonymous\", \"function\" or \"admin\""))
	}

	return result
}

func isValidFunctionAppName(functionAppName string) bool {
	// Valid characters are `a-z`, `0-9`, and `-`.
	return regexp.MustCompile("^[[:alpha:][:digit:]-]+$").MatchString(functionAppName)
}

func isValidFunctionName(functionName string) bool {
	// The name must be unique within a Function App. It must start with a letter and can contain
	// letters, numbers (0-9), dashes ("-"), and underscores ("_").
	return regexp.MustCompile("^[[:alpha:]][[:alpha:][:digit:]_-]*$").MatchString(functionName)
}

func isValidAuthLevel(authLevel string) bool {
	// The authorization level can be one of the following values:
	// * "anonymous" - No API key is required.
	// * "function" - A function-specific API key is required. This is the default value if none is
	//                provided.
	// * "admin" - The master key is required.
	for _, str := range []string{"anonymous", "function", "admin"} {
		if authLevel == str {
			return true
		}
	}
	return false
}
