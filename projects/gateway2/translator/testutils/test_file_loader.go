package testutils

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/protoutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/solo-io/gloo/projects/gateway2/controller/scheme"
	"github.com/solo-io/gloo/projects/gateway2/translator/listener"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/skv2/contrib/pkg/sets"
)

var (
	NoFilesFound = errors.New("no k8s files found")
)

func LoadFromFiles(ctx context.Context, filename string) ([]client.Object, error) {

	fileOrDir, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}

	var yamlFiles []string
	if fileOrDir.IsDir() {
		contextutils.LoggerFrom(ctx).Infof("looking for YAML files in directory tree rooted at: %s", fileOrDir.Name())
		err := filepath.WalkDir(filename, func(path string, d fs.DirEntry, _ error) error {
			if strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml") {
				yamlFiles = append(yamlFiles, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		yamlFiles = append(yamlFiles, filename)
	}

	if len(yamlFiles) == 0 {
		return nil, NoFilesFound
	}

	contextutils.LoggerFrom(ctx).Infow("user configuration YAML files found", zap.Strings("files", yamlFiles))

	var resources []client.Object
	for _, file := range yamlFiles {
		objs, err := parseFile(ctx, file)
		if err != nil {
			return nil, err
		}

		for _, obj := range objs {
			clientObj, ok := obj.(client.Object)
			if !ok {
				return nil, errors.Errorf("cannot convert runtime.Object to client.Object: %+v", obj)
			}
			if clientObj.GetNamespace() == "" {
				// fill in default namespace
				clientObj.SetNamespace("default")
			}
			resources = append(resources, clientObj)
		}

	}

	return resources, nil
}

func parseFile(ctx context.Context, filename string) ([]runtime.Object, error) {
	scheme := scheme.NewScheme()
	file, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	type metaOnly struct {
		metav1.TypeMeta   `json:",inline"`
		metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	}

	// Split into individual YAML documents
	resourceYamlStrings := bytes.Split(file, []byte("\n---\n"))

	// Create resources from YAML documents
	var genericResources []runtime.Object
	for _, objYaml := range resourceYamlStrings {

		// Skip empty documents
		if len(bytes.TrimSpace(objYaml)) == 0 {
			continue
		}

		var meta metaOnly
		if err := yaml.Unmarshal(objYaml, &meta); err != nil {
			contextutils.LoggerFrom(ctx).Warnw("failed to parse resource metadata, skipping YAML document",
				zap.String("filename", filename),
				zap.String("truncatedYamlDoc", truncateString(string(objYaml), 100)),
			)
			continue
		}

		gvk := schema.FromAPIVersionAndKind(meta.APIVersion, meta.Kind)
		obj, err := scheme.New(gvk)
		if err != nil {
			contextutils.LoggerFrom(ctx).Warnw("unknown resource kind",
				zap.String("filename", filename),
				zap.String("resourceKind", gvk.String()),
				zap.String("truncatedYamlDoc", truncateString(string(objYaml), 100)),
			)
			continue
		}
		if err := yaml.Unmarshal(objYaml, obj); err != nil {
			contextutils.LoggerFrom(ctx).Warnw("failed to parse resource YAML",
				zap.String("filename", filename),
				zap.String("resourceKind", gvk.String()),
				zap.String("resourceId", sets.Key(obj.(client.Object))),
				zap.String("truncatedYamlDoc", truncateString(string(objYaml), 100)),
			)
			continue
		}

		genericResources = append(genericResources, obj)
	}

	return genericResources, err
}

func truncateString(str string, num int) string {
	result := str
	if len(str) > num {
		result = str[0:num] + "..."
	}
	return result
}

func ReadProxyFromFile(filename string) ([]listener.ListenerAndRoutes, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, eris.Wrapf(err, "reading proxy file")
	}
	var proxy v1.Proxy
	if err := protoutils.UnmarshalYaml(data, &proxy); err != nil {
		return nil, eris.Wrapf(err, "parsing proxy from file")
	}
	return &proxy, nil
}

func MarshalYaml(m proto.Message) ([]byte, error) {
	jsn, err := protoutils.MarshalBytes(m)
	if err != nil {
		return nil, err
	}
	return yaml.JSONToYAML(jsn)
}
