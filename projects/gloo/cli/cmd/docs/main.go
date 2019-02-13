package main

import (
	"bytes"
	"log"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

const FrontMatter = `---
title: "{{ replace .Name "_" " " }}"
weight: 5
---
`

var funcMap = template.FuncMap{
	"title":   strings.Title,
	"replace": func(s, old, new string) string { return strings.Replace(s, old, new, -1) },
}

var FrontMatterTemplate = template.Must(template.New("frontmatter").Funcs(funcMap).Parse(FrontMatter))

func renderFrontMatter(filename string) string {
	_, justFilename := filepath.Split(filename)
	ext := filepath.Ext(justFilename)
	justFilename = justFilename[:len(justFilename)-len(ext)]
	info := struct {
		Name string
	}{
		Name: justFilename,
	}
	var buf bytes.Buffer
	err := FrontMatterTemplate.Execute(&buf, info)
	if err != nil {
		panic(err)
	}
	return buf.String()
}

func main() {
	app := cmd.GlooCli(version.Version)
	disableAutoGenTag(app)
	//emptyStr := func(s string) string { return "" }
	linkHandler := func(s string) string {
		if strings.HasSuffix(s, ".md") {
			return filepath.Join("..", s[:len(s)-3])
		}
		return s
	}
	err := doc.GenMarkdownTreeCustom(app, "./docs/cli", renderFrontMatter, linkHandler)
	if err != nil {
		log.Fatal(err)
	}
}

func disableAutoGenTag(c *cobra.Command) {
	c.DisableAutoGenTag = true
	for _, c := range c.Commands() {
		disableAutoGenTag(c)
	}
}
