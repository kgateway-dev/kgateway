package compress

import (
	bytes "bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	v1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
)

const (
	compressedSpec  = "compressedSpec"
	compressed_spec = "compressed_spec"
	compressedKey   = "gloo.solo.io/compress"
	compressedValue = "true"
)

func IsCompressed(in v1.Spec) bool {
	_, ok1 := in[compressedSpec]
	_, ok2 := in[compressed_spec]
	return ok1 || ok2
}

func ShouldCompress(in resources.Resource) bool {
	annotations := in.GetMetadata().Annotations
	if annotations == nil {
		return false
	}

	return annotations[compressedKey] == compressedValue
}

func SetShouldCompressed(in resources.InputResource) {
	metadata := in.GetMetadata()
	annotations := metadata.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[compressedKey] = compressedValue
	metadata.Annotations = annotations
	in.SetMetadata(metadata)
}

func CompressSpec(s v1.Spec) (v1.Spec, error) {
	// serialize  spec to json:
	ser, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	w.Write(ser)
	w.Close()

	newSpec := v1.Spec{}
	newSpec[compressedSpec] = b.Bytes()
	return newSpec, nil
}

func UncompressSpec(s v1.Spec) (v1.Spec, error) {

	compressed, ok := s[compressedSpec]
	if !ok {
		compressed, ok = s[compressed_spec]
		if !ok {
			return nil, fmt.Errorf("not compressed")
		}
	}

	var spec v1.Spec
	switch data := compressed.(type) {
	case []byte:
		err := json.Unmarshal(data, &spec)
		if err != nil {
			return nil, crd.MarshalErr(err, "data not json")
		}

		return spec, nil
	case string:
		decodedData, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, crd.MarshalErr(err, "data not base64")
		}

		var b bytes.Buffer
		r, err := zlib.NewReader(bytes.NewBuffer(decodedData))
		io.Copy(&b, r)
		r.Close()

		err = json.Unmarshal(b.Bytes(), &spec)
		if err != nil {
			return nil, crd.MarshalErr(err, "data is not valid json")
		}
		return spec, nil
	default:
		return nil, fmt.Errorf("unknown datatype %T", compressed)
	}
}
