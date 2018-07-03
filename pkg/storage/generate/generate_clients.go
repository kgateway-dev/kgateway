package main

import (
	"bytes"
	"flag"
	"io/ioutil"
	"path/filepath"
	"text/template"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/log"
)

type clientType struct {
	FilenamePrefix      string
	LowercaseName       string
	LowercasePluralName string
	UppercaseName       string
	UppercasePluralName string
}

var clients = []clientType{
	{
		FilenamePrefix:      "upstreams",
		LowercaseName:       "upstream",
		LowercasePluralName: "upstreams",
		UppercaseName:       "Upstream",
		UppercasePluralName: "Upstreams",
	},
	{
		FilenamePrefix:      "virtual_services",
		LowercaseName:       "virtualService",
		LowercasePluralName: "virtualServices",
		UppercaseName:       "VirtualService",
		UppercasePluralName: "VirtualServices",
	},
	{
		FilenamePrefix:      "roles",
		LowercaseName:       "role",
		LowercasePluralName: "roles",
		UppercaseName:       "Role",
		UppercasePluralName: "Roles",
	},
	{
		FilenamePrefix:      "attributes",
		LowercaseName:       "attribute",
		LowercasePluralName: "attributes",
		UppercaseName:       "Attribute",
		UppercasePluralName: "Attributes",
	},
}

func main() {
	inputFile := flag.String("f", "", "input client template")
	outputDirectory := flag.String("o", "", "output directory for client files")
	flag.Parse()
	if *inputFile == "" || *outputDirectory == "" {
		log.Fatalf("must specify -f and -o")
	}
	if err := writeClientTemplates(*inputFile, *outputDirectory); err != nil {
		log.Fatalf("failed generating client templates: %s", err.Error())
	}
	log.Printf("success")
}

func writeClientTemplates(inputFile, outputDir string) error {
	fileName := filepath.Base(inputFile)
	for _, client := range clients {
		tmpl, err := template.New("Test_Resources").ParseFiles(inputFile)
		if err != nil {
			return errors.Wrap(err, "parsing template from "+inputFile)
		}

		buf := &bytes.Buffer{}
		if err := tmpl.ExecuteTemplate(buf, fileName, client); err != nil {
			return errors.Wrap(err, "executing template")
		}

		err = ioutil.WriteFile(filepath.Join(outputDir, client.FilenamePrefix+".go"), buf.Bytes(), 0644)
		if err != nil {
			return errors.Wrap(err, "writing generated client bytes")
		}
	}
	return nil
}
