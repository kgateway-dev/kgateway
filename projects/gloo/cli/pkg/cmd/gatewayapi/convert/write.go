package convert

import (
	"fmt"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/gatewayapi/convert/domain"
	"os"
	"path/filepath"
	"sigs.k8s.io/yaml"
	"strings"
)

func (g *GatewayAPIOutput) Write(opts *Options) error {

	if folderExists(opts.OutputDir) {
		if !opts.DeleteOutputDir {
			return fmt.Errorf("output-dir %s already exists, not writing files", opts.OutputDir)
		}
		if err := os.RemoveAll(opts.OutputDir); err != nil {
			return err
		}
	}

	//create the output dir
	if err := os.MkdirAll(opts.OutputDir, os.ModePerm); err != nil {
		return err
	}

	// TODO we need to know all the files we are going to write a head of time because we want to wipe

	var err error
	for _, r := range g.gatewayAPICache.Gateways {
		yml, err := yaml.Marshal(r.Gateway)
		if err != nil {
			return err
		}
		if err := writeObjectToFile(opts, r, yml); err != nil {
			return err
		}
	}
	// Write Routes
	for _, r := range g.gatewayAPICache.HTTPRoutes {
		yml, err := yaml.Marshal(r.HTTPRoute)
		if err != nil {
			return err
		}
		if err := writeObjectToFile(opts, r, yml); err != nil {
			return err
		}
	}
	for _, r := range g.gatewayAPICache.RouteOptions {
		yml, err := yaml.Marshal(r.RouteOption)
		if err != nil {
			return err
		}
		if err := writeObjectToFile(opts, r, yml); err != nil {
			return err
		}
	}
	for _, r := range g.gatewayAPICache.VirtualHostOptions {
		yml, err := yaml.Marshal(r.VirtualHostOption)
		if err != nil {
			return err
		}
		if err := writeObjectToFile(opts, r, yml); err != nil {
			return err
		}
	}
	for _, r := range g.gatewayAPICache.ListenerOptions {
		yml, err := yaml.Marshal(r.ListenerOption)
		if err != nil {
			return err
		}
		if err := writeObjectToFile(opts, r, yml); err != nil {
			return err
		}
	}
	for _, r := range g.gatewayAPICache.HTTPListenerOptions {
		yml, err := yaml.Marshal(r.HttpListenerOption)
		if err != nil {
			return err
		}
		if err := writeObjectToFile(opts, r, yml); err != nil {
			return err
		}
	}
	for _, r := range g.gatewayAPICache.Upstreams {
		yml, err := yaml.Marshal(r.Upstream)
		if err != nil {
			return err
		}
		if err := writeObjectToFile(opts, r, yml); err != nil {
			return err
		}
	}
	for _, r := range g.gatewayAPICache.AuthConfigs {
		yml, err := yaml.Marshal(r.AuthConfig)
		if err != nil {
			return err
		}
		if err := writeObjectToFile(opts, r, yml); err != nil {
			return err
		}
	}
	for _, r := range g.gatewayAPICache.ListenerSets {
		yml, err := yaml.Marshal(r.XListenerSet)
		if err != nil {
			return err
		}
		if err := writeObjectToFile(opts, r, yml); err != nil {
			return err
		}
	}
	for _, r := range g.gatewayAPICache.YamlObjects {
		yml, err := yaml.Marshal(r.Object)
		if err != nil {
			return err
		}
		if err := writeObjectToFile(opts, r, yml); err != nil {
			return err
		}
	}
	// create Gloo Errors directory and splt the files based on their error string.
	folder, err := createSubDir(opts.OutputDir, "gloo-errors")
	if err != nil {
		return err
	}
	//organize all errors into a map

	for t, errors := range g.errors {
		f, err := os.Create(fmt.Sprintf("%s/%s.txt", folder, t))
		if err != nil {
			return err
		}
		errorsMap := make(map[string][]error)
		for _, err := range errors {
			// for each error organize by type and name string
			key := fmt.Sprintf("[%s] %s/%s", err.crdType, err.namespace, err.name)
			_, exist := errorsMap[key]
			if !exist {
				errorsMap[key] = make([]error, 0)
			}
			errorsMap[key] = append(errorsMap[key], err.err)
		}
		for key, errors := range errorsMap {
			_, err := f.WriteString("\n" + key + "\n")
			if err != nil {
				return err
			}
			for _, gerr := range errors {
				_, err := f.WriteString("\t" + gerr.Error() + "\n")
				if err != nil {
					return err
				}
			}
		}

		err = f.Close()
		if err != nil {
			return err
		}
	}
	if len(g.errors) > 0 {
		fmt.Printf("Errros were encountered during translation, please check %s/gloo-errors\n", opts.OutputDir)
	}

	return nil
}

func writeObjectToFile(opts *Options, wrapper domain.Wrapper, stringBytes []byte) error {
	splitFilesByNamespace := !opts.RetainFolderStructure
	var err error
	if splitFilesByNamespace {
		_, err = createSubDir(opts.OutputDir, wrapper.GetNamespace())
		if err != nil {
			return err
		}
	}
	if opts.RetainFolderStructure {
		// retain original file name
		if err := appendToFile(opts.OutputDir, wrapper.GetOriginalFileName(), stringBytes); err != nil {
			return err
		}
	} else {
		// by file per namespace
		fileName := fmt.Sprintf("%s-%s.yaml", wrapper.GetObjectKind().GroupVersionKind().Kind, wrapper.GetName())
		if err := appendToFile(filepath.Join(opts.OutputDir, wrapper.GetNamespace()), fileName, stringBytes); err != nil {
			return err
		}
	}
	return nil
}

func appendToFile(outputDir string, filename string, yml []byte) error {
	filePath := filepath.Join(outputDir, filename)
	if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
		return err
	}
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err = f.WriteString(fmt.Sprintf("\n---\n%s", removeNullYamlFields(yml))); err != nil {
		return err
	}

	return nil
}

func createSubDir(outputDir string, subDir string) (string, error) {
	folder := filepath.Join(outputDir, subDir)
	err := os.MkdirAll(folder, os.ModePerm)
	if err != nil {
		return "", err
	}
	return folder, nil
}

func removeNullYamlFields(yamlData []byte) string {
	stringData := strings.ReplaceAll(string(yamlData), "  creationTimestamp: null\n", "")
	stringData = strings.ReplaceAll(stringData, "status:\n", "")
	stringData = strings.ReplaceAll(stringData, "parents: null\n", "")
	stringData = strings.ReplaceAll(stringData, "status: {}\n", "")
	stringData = strings.ReplaceAll(stringData, "\n\n\n", "\n")
	stringData = strings.ReplaceAll(stringData, "\n\n", "\n")
	stringData = strings.ReplaceAll(stringData, "spec: {}\n", "")
	stringData = strings.ReplaceAll(stringData, "  listeners: null\n", "")
	return stringData
}
