package main

import (
	"flag"
	"github.com/solo-io/gloo/test/helpers"
	"path/filepath"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/pkg/errors"
	"bytes"
	"io/ioutil"
	"text/template"
)

type clientType struct {
	FilenamePrefix string
	LowercaseName       string
	LowercasePluralName string
	UppercaseName       string
	UppercasePluralName string
}

var clients = []clientType{
	{
		FilenamePrefix: "upstreams",
		LowercaseName:       "upstream",
		LowercasePluralName: "upstreams",
		UppercaseName:       "Upstream",
		UppercasePluralName: "Upstreams",
	},
	{
		FilenamePrefix: "virtual_services",
		LowercaseName:       "virtualService",
		LowercasePluralName: "virtualServices",
		UppercaseName:       "VirtualService",
		UppercasePluralName: "VirtualServices",
	},
}

func main() {
	baseDir := filepath.Join(helpers.GlooSoloDirectory(), "pkg", "storage", "crd")
	inputFile := flag.String("f", filepath.Join(baseDir, "solo.io", "client_template.go.tmpl"), "input client template")
	outputDirectory := flag.String("o", baseDir, "output directory for client files")
	flag.Parse()
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
