package aws

import (
	"errors"
	"fmt"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/protoutil"
)

var (
	ValidRegions = map[string]bool{
		"us-east-2":      true,
		"us-east-1":      true,
		"us-west-1":      true,
		"us-west-2":      true,
		"ap-northeast-1": true,
		"ap-northeast-2": true,
		"ap-south-1":     true,
		"ap-southeast-1": true,
		"ap-southeast-2": true,
		"ca-central-1":   true,
		"cn-north-1":     true,
		"eu-central-1":   true,
		"eu-west-1":      true,
		"eu-west-2":      true,
		"eu-west-3":      true,
		"sa-east-1":      true,
	}
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
	return s, s.validateLambda()
}

func (s *UpstreamSpec) validateLambda() error {
	_, exists := ValidRegions[s.Region]
	if !exists {
		return errors.New("no such region")
	}

	if s.SecretRef == "" {
		return errors.New("missing secret reference")
	}

	return nil
}

func (s *UpstreamSpec) GetLambdaHostname() string {
	return fmt.Sprintf("lambda.%s.amazonaws.com", s.Region)
}

type FunctionSpec struct {
	FunctionName string `json:"function_name"`
	Qualifier    string `json:"qualifier"`
}

func DecodeFunctionSpec(generic v1.FunctionSpec) (*FunctionSpec, error) {
	s := new(FunctionSpec)
	if err := protoutil.UnmarshalStruct(generic, s); err != nil {
		return nil, err
	}
	return s, s.ValidateLambda()
}

func EncodeFunctionSpec(spec FunctionSpec) *types.Struct {
	v1Spec, err := protoutil.MarshalStruct(spec)
	if err != nil {
		panic(err)
	}
	return v1Spec
}

func (s *FunctionSpec) ValidateLambda() error {
	if s.FunctionName == "" {
		return errors.New("invalid function name")
	}
	return nil
}
