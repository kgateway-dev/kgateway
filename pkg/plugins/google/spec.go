package gfunc

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/protoutil"
)

var (
	ValidRegions = map[string]bool{
		"northamerica-northeast1": true,
		"us-central1":             true,
		"us-west1":                true,
		"us-east4":                true,
		"us-east1":                true,
		"southamerica-east1":      true,
		"europe-west1":            true,
		"europe-west2":            true,
		"europe-west3":            true,
		"europe-west4":            true,
		"asia-south1":             true,
		"asia-southeast1":         true,
		"asia-east1":              true,
		"asia-northeast1":         true,
		"australia-southeast1":    true,
	}
)

type UpstreamSpec struct {
	Region    string `json:"region"`
	ProjectId string `json:"project_id"`
}

func DecodeUpstreamSpec(generic v1.UpstreamSpec) (*UpstreamSpec, error) {
	s := new(UpstreamSpec)
	if err := protoutil.UnmarshalStruct(generic, s); err != nil {
		return nil, err
	}
	return s, s.validateUpstream()
}

func (s *UpstreamSpec) validateUpstream() error {
	_, exists := ValidRegions[s.Region]
	if !exists {
		return errors.New("no such region")

	}

	if s.ProjectId == "" {
		return errors.New("missing project ID")
	}
	return nil
}

func (s *UpstreamSpec) GetGFuncHostname() string {
	return fmt.Sprintf("%s-%s.cloudfunctions.net", s.Region, s.ProjectId)
}

type FunctionSpec struct {
	URL string `json:"URL"`

	path string // used by envoy filter
	host string // used by envoy filter
}

func DecodeFunctionSpec(generic v1.FunctionSpec) (*FunctionSpec, error) {
	s := new(FunctionSpec)
	if err := protoutil.UnmarshalStruct(generic, s); err != nil {
		return nil, err
	}
	if err := s.ValidateGFunc(); err != nil {
		return s, err
	}
	parsedURL, _ := url.Parse(s.URL)
	s.path = parsedURL.Path
	s.host = parsedURL.Host
	return s, nil
}

func EncodeFunctionSpec(spec FunctionSpec) *types.Struct {
	v1Spec, err := protoutil.MarshalStruct(spec)
	if err != nil {
		panic(err)
	}
	return v1Spec
}

// TODO(ashish) - is this being called from outside this package?
// can this be merged into DecodeFuncionSpec
func (s *FunctionSpec) ValidateGFunc() error {
	if s.URL == "" {
		return errors.New("invalid function URL")
	}
	parsedURL, err := url.Parse(s.URL)
	if err != nil {
		return err
	}

	if parsedURL.Path == "" {
		return errors.New("invalid function URL; missing path")
	}

	if parsedURL.Host == "" {
		return errors.New("invalid function URL; missing host")
	}
	return nil
}

func NewFuncFromUrl(funcurl string) (*FunctionSpec, error) {
	s := new(FunctionSpec)
	s.URL = funcurl
	parsedUrl, err := url.Parse(funcurl)
	if err != nil {
		return s, err
	}

	s.path = parsedUrl.Path
	s.host = parsedUrl.Host
	return s, nil
}
