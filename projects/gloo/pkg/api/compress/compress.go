package compress

import (
	bytes "bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"io"
	"strconv"

	"github.com/solo-io/solo-kit/pkg/utils/specutils"

	"github.com/rotisserie/eris"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	v1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
)

const (
	CompressedKey           = "gloo.solo.io/compress"
	CompressedValue         = "true"
	compressedSpec          = "compressedSpec"
	compressed_spec         = "compressed_spec"
	ShortenKey              = "gloo.solo.io/maxStatusSize"
	MaxLengthWarningMessage = "\nStatus truncated, see logs for details."
)

func isCompressed(in v1.Spec) bool {
	_, ok1 := in[compressedSpec]
	_, ok2 := in[compressed_spec]
	return ok1 || ok2
}

func shouldCompress(in resources.Resource) bool {
	annotations := in.GetMetadata().GetAnnotations()
	if annotations == nil {
		return false
	}

	return annotations[CompressedKey] == CompressedValue
}
func addAnnotation(in resources.Resource, key string, value string) {
	metadata := &core.Metadata{}
	if in.GetMetadata() != nil {
		metadata = in.GetMetadata()
	}
	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[key] = value
	metadata.Annotations = annotations
	in.SetMetadata(metadata)
}
func SetShouldCompressed(in resources.Resource) {
	addAnnotation(in, CompressedKey, CompressedValue)
}
func GetMaxStatusSizeBytes(in resources.Resource) int64 {
	annotations := in.GetMetadata().GetAnnotations()
	if annotations == nil {
		return -1
	}
	if maxSize, ok := annotations[ShortenKey]; ok {
		// We ensure that we can parse the size before setting this value
		size, _ := strconv.ParseInt(maxSize, 0, 64)
		return size
	}
	return -1
}
func SetMaxStatusSizeBytes(in resources.Resource, maxStatusSize string) error {
	if _, err := strconv.ParseInt(maxStatusSize, 0, 64); err != nil {
		return err
	}
	addAnnotation(in, ShortenKey, maxStatusSize)
	return nil
}
func compressSpec(s v1.Spec) (v1.Spec, error) {
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

func uncompressSpec(s v1.Spec) (v1.Spec, error) {

	compressed, ok := s[compressedSpec]
	if !ok {
		compressed, ok = s[compressed_spec]
		if !ok {
			return nil, eris.Errorf("not compressed")
		}
	}

	var compressedData []byte
	var spec v1.Spec
	switch data := compressed.(type) {
	case []byte:
		compressedData = data
	case string:
		decodedData, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, crd.MarshalErr(err, "data not base64")
		}
		compressedData = decodedData

	default:
		return nil, eris.Errorf("unknown datatype %T", compressed)
	}

	var b bytes.Buffer
	r, err := zlib.NewReader(bytes.NewBuffer(compressedData))
	if err != nil {
		return nil, eris.Wrap(err, "error creating buffer")
	}
	defer r.Close()
	_, err = io.Copy(&b, r)
	if err != nil {
		return nil, eris.Wrap(err, "error copying buffer")
	}

	err = json.Unmarshal(b.Bytes(), &spec)
	if err != nil {
		return nil, crd.MarshalErr(err, "data is not valid json")
	}
	return spec, nil
}

func UnmarshalSpec(in resources.Resource, spec v1.Spec) error {
	if isCompressed(spec) {
		var err error
		spec, err = uncompressSpec(spec)
		if err != nil {
			return eris.Wrapf(err, "reading unmarshalling spec on resource %v in namespace %v into %T", in.GetMetadata().GetName(), in.GetMetadata().GetNamespace(), in)
		}
		// if we have a compressed spec, make sure the resource is marked for compression
		SetShouldCompressed(in)
	}
	if err := specutils.UnmarshalSpecMapToResource(spec, in); err != nil {
		return eris.Wrapf(err, "reading crd spec on resource %v in namespace %v into %T", in.GetMetadata().GetName(), in.GetMetadata().GetNamespace(), in)
	}

	return nil
}

func MarshalSpec(in resources.Resource) (v1.Spec, error) {

	data, err := protoutils.MarshalMap(in)
	if err != nil {
		return nil, crd.MarshalErr(err, "resource to map")
	}

	delete(data, "metadata")
	delete(data, "namespacedStatuses")
	// save this as usual:
	var spec v1.Spec
	spec = data
	if shouldCompress(in) {
		spec, err = compressSpec(spec)
		if err != nil {
			return nil, eris.Wrapf(err, "reading marshalling spec on resource %v in namespace %v into %T", in.GetMetadata().GetName(), in.GetMetadata().GetNamespace(), in)
		}
	}
	return spec, nil
}

func UnmarshalStatus(in resources.InputResource, status v1.Status, unmarshaler resources.StatusUnmarshaler) {
	unmarshaler.UnmarshalStatus(status, in)
}

func MarshalStatus(in resources.InputResource) (v1.Status, error) {
	namespacedStatuses := in.GetNamespacedStatuses()
	if namespacedStatuses == nil {
		return v1.Status{}, nil
	}
	maxStatusSize := GetMaxStatusSizeBytes(in)
	if maxStatusSize > 0 {
		namespacedStatuses = shortenStatus(namespacedStatuses, maxStatusSize)
	}
	namespacedStatusMap, err := protoutils.MarshalMapFromProtoWithEnumsAsInts(namespacedStatuses)
	if err != nil {
		return nil, crd.MarshalErr(err, "resource status to map")
	}
	return namespacedStatusMap, nil
}

// deprecated and will be removed in future releases.
// see https://github.com/solo-io/solo-kit/pull/524 and https://github.com/solo-io/solo-kit/pull/525 for current behavior
func shortenStatus(namespacedStatuses *core.NamespacedStatuses, maxLength int64) *core.NamespacedStatuses {
	statuses := namespacedStatuses.GetStatuses()
	for ns, status := range namespacedStatuses.GetStatuses() {
		if int64(len(status.GetReason())) > maxLength {
			status.Reason = status.GetReason()[0:maxLength] + MaxLengthWarningMessage
			statuses[ns] = status
		}
	}
	namespacedStatuses.Statuses = statuses
	return namespacedStatuses
}
