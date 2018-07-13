package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"html/template"
	"io/ioutil"
	"log"
	"path/filepath"

	"os"

	"strings"

	"github.com/ilackarms/protoc-gen-doc"
	"github.com/pkg/errors"
)

func main() {
	f := flag.String("f", os.Getenv("GOPATH")+"/src/github.com/solo-io/gloo/docs/"+"api.json", "input json file")
	tmplFile := flag.String("t", os.Getenv("GOPATH")+"/src/github.com/solo-io/gloo/docs/markdown.tmpl", "template to build from")
	outDir := flag.String("o", os.Getenv("GOPATH")+"/src/github.com/solo-io/gloo/docs/v1/", "output dir")
	flag.Parse()
	if err := run(*f, *tmplFile, *outDir); err != nil {
		log.Fatal(err)
	}
}

func run(file, tmplFile, outDir string) error {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return errors.Wrapf(err, "reading input file")
	}
	var protoDescriptor gendoc.Template
	err = json.Unmarshal(data, &protoDescriptor)
	if err != nil {
		return errors.Wrapf(err, "unmarshalling proto descriptor")
	}

	inputTemplate, err := ioutil.ReadFile(tmplFile)
	if err != nil {
		return errors.Wrapf(err, "reading tmpl file")
	}

	fixMapEntryKludge(&protoDescriptor)
	getFilesForTypes(&protoDescriptor)

	for _, protoFile := range protoDescriptor.Files {
		protoFile.Name = strings.TrimSuffix(protoFile.Name, ".proto")
		log.Printf("%s", protoFile.Name)
		tmpl, err := template.New("Proto Doc Template").Funcs(map[string]interface{}{
			"p":           gendoc.PFilter,
			"para":        gendoc.ParaFilter,
			"nobr":        gendoc.NoBrFilter,
			"yamlType":    yamlType,
			"noescape":    noEscape,
			"linkForType": linkForType,
		}).Parse(string(inputTemplate))
		if err != nil {
			return errors.Wrapf(err, "parsing template")
		}
		var buf bytes.Buffer
		err = tmpl.Execute(&buf, protoFile)
		if err != nil {
			return errors.Wrapf(err, "executing template")
		}
		name := protoFile.Name
		// plugins go in a special subdir
		if strings.Contains(name, "plugins/") {
			name = filepath.Join("plugins", strings.TrimPrefix(strings.Replace(protoFile.Name, "/", "_", -1), "github.com_solo-io_gloo_pkg_"))
			os.MkdirAll(outDir+"/plugins", 0755)

		}
		err = ioutil.WriteFile(outDir+"/"+name+".md", buf.Bytes(), 0644)
		if err != nil {
			return errors.Wrapf(err, "writing output md")
		}
	}
	return nil
}

var filesForTypes = make(map[string]string)

func getFilesForTypes(protoDescriptor *gendoc.Template) {
	for _, protoFile := range protoDescriptor.Files {
		for _, message := range protoFile.Messages {
			for _, field := range message.Fields {
				filesForTypes[field.FullType] = strings.TrimSuffix(protoFile.Name, ".proto")
			}
		}
	}
	// overwrite for status and metadata. hacky but the fastest solution!
	for _, protoFile := range protoDescriptor.Files {
		for _, message := range protoFile.Messages {
			filesForTypes[message.FullName] = strings.TrimSuffix(protoFile.Name, ".proto")
		}
	}
}

type mapEntry struct {
	key   *gendoc.MessageField
	value *gendoc.MessageField
}

func fixMapEntryKludge(protoDescriptor *gendoc.Template) {
	mapEntriesToFix := make(map[string]mapEntry)
	for _, protoFile := range protoDescriptor.Files {
		messages := protoFile.Messages
		protoFile.Messages = nil
		// remove "entry" types, we are converting these back to map<string, string>
		for _, message := range messages {
			if strings.HasSuffix(message.Name, "Entry") {
				if len(message.Fields) != 2 {
					log.Fatalf("bad assumption: %#v is not a map entry, or doesn't have 2 fields", message)
				}
				mapEntriesToFix[message.Name] = mapEntry{key: message.Fields[0], value: message.Fields[1]}
			} else {
				protoFile.Messages = append(protoFile.Messages, message)
			}
		}
	}
	for _, protoFile := range protoDescriptor.Files {
		for _, message := range protoFile.Messages {
			for _, field := range message.Fields {
				if entry, ok := mapEntriesToFix[field.Type]; ok {
					field.Type = "map<" + entry.key.Type + "," + entry.value.Type + ">"
					field.FullType = "map<" + entry.key.FullType + "," + entry.value.FullType + ">"
					field.LongType = "map<" + entry.key.LongType + "," + entry.value.LongType + ">"
					field.Label = ""
					log.Printf("changed field %v", field.Name)
				}
			}
		}
	}
}

func yamlType(longType, label string) string {
	yamlType := func() string {
		if strings.HasPrefix(longType, "map<") {
			return longType
		}
		switch longType {
		case "string":
			fallthrough
		case "uint32":
			fallthrough
		case "bool":
			fallthrough
		case "int32":
			return longType
		case "Status":
			return "(read only)"
		}
		return "{" + longType + "}"
	}()
	if label == "repeated" {
		yamlType = "[" + yamlType + "]"
	}
	return yamlType
}

func noEscape(s string) template.HTML {
	return template.HTML(s)
}

func linkForType(longType, fullType string) string {
	if !isObjectType(longType) {
		return longType //no linking for primitives
	}
	var link string
	switch {
	case longType == "google.protobuf.Duration":
		link = "https://developers.google.com/protocol-buffers/docs/reference/csharp/class/google/protobuf/well-known-types/duration"
	case longType == "google.protobuf.Struct":
		link = "https://developers.google.com/protocol-buffers/docs/reference/csharp/class/google/protobuf/well-known-types/struct"
	default:
		link = filesForTypes[fullType] + ".md#" + fullType
	}
	return "[" + longType + "](" + link + ")"
}

func isObjectType(longType string) bool {
	if strings.HasPrefix(longType, "map<") {
		return false
	}
	switch longType {
	case "string":
		fallthrough
	case "uint32":
		fallthrough
	case "bool":
		fallthrough
	case "int32":
		return false
	}
	return true
}
