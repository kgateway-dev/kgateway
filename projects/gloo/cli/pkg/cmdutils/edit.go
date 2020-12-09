package cmdutils

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/ghodss/yaml"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/protoutils"
	"k8s.io/kubectl/pkg/cmd/util/editor"
)

type EditFunc func(prefix, suffix string, r io.Reader) ([]byte, string, error)

var EditFileForTest EditFunc

type Editor struct {
	JsonTransform func([]byte) []byte
}

func defaultEdit(prefix, suffix string, r io.Reader) ([]byte, string, error) {
	edit := editor.NewDefaultEditor([]string{"EDITOR"})
	return edit.LaunchTempFile(fmt.Sprintf("%s-edit-", filepath.Base(os.Args[0])), ".yaml", r)
}

func (e *Editor) EditConfig(m proto.Message) (proto.Message, error) {
	var buf bytes.Buffer

	err := e.PrintYAML(m, &buf)
	if err != nil {
		return nil, err
	}

	curEditFunc := defaultEdit
	if EditFileForTest != nil {
		curEditFunc = EditFileForTest
	}

	edited, file, err := curEditFunc(fmt.Sprintf("%s-edit-", filepath.Base(os.Args[0])), ".yaml", &buf)
	if err != nil {
		return nil, err
	}
	if file != "" {
		os.Remove(file)
	}

	result := Fresh(m)
	err = e.ReadDescriptors(edited, result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (e *Editor) PrintYAML(m proto.Message, w io.Writer) error {
	jsn, err := protoutils.MarshalBytes(m)
	if err != nil {
		return errors.Wrap(err, "unable to marshal")
	}
	b, err := yaml.JSONToYAML(jsn)
	if err != nil {
		return errors.Wrap(err, "unable to convert to YAML")
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

func (e *Editor) ReadDescriptors(data []byte, result proto.Message) error {
	jsn, err := yaml.YAMLToJSON(data)
	if err != nil {
		return err
	}

	if e.JsonTransform != nil {
		jsn = e.JsonTransform(jsn)
	}

	return jsonpb.Unmarshal(bytes.NewBuffer(jsn), result)
}

func Fresh(src proto.Message) proto.Message {
	in := reflect.ValueOf(src)
	if in.IsNil() {
		return src
	}
	out := reflect.New(in.Type().Elem())
	return out.Interface().(proto.Message)
}
