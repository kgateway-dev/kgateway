package swagger

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-openapi/spec"
	transformation_plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	"github.com/solo-io/go-utils/log"
)

func createFunctionsForPath(pathFunctions map[string]*transformation_plugins.TransformationTemplate, basePath, functionPath string, path spec.PathItemProps, definitions spec.Definitions) {
	appendFunction := func(method string, operation *spec.Operation) {
		name, trans := createFunctionForOpertaion(method, basePath, functionPath, operation.OperationProps, definitions)
		pathFunctions[name] = trans
	}
	if path.Get != nil {
		appendFunction("GET", path.Get)
	}
	if path.Put != nil {
		appendFunction("PUT", path.Put)
	}
	if path.Post != nil {
		appendFunction("POST", path.Post)
	}
	if path.Delete != nil {
		appendFunction("DELETE", path.Delete)
	}
	if path.Options != nil {
		appendFunction("OPTIONS", path.Options)
	}
	if path.Head != nil {
		appendFunction("HEAD", path.Head)
	}
	if path.Patch != nil {
		appendFunction("PATCH", path.Patch)
	}
}

func createFunctionForOpertaion(method string, basePath, functionPath string, operation spec.OperationProps, definitions spec.Definitions) (string, *transformation_plugins.TransformationTemplate) {
	var queryParams, headerParams []string
	//bodyParams := make(map[string]spec.SchemaProps)
	var body *string
	for _, param := range operation.Parameters {
		// sort parameters by the template they will go into
		switch param.In {
		case "query":
			queryParams = append(queryParams, fmt.Sprintf("%v={{default(%v, \"\")}}", param.Name, param.Name))
		case "header":
			headerParams = append(headerParams, param.Name)
		case "path":
			// nothing to do here, we already get the template
		case "formData":
			log.Warnf("form data params not currently supported; ignoring")
		case "body":
			tmp := getBodyTemplate("", definitions[param.Name].SchemaProps, definitions)
			body = &tmp
			//bodyParams[param.Name] = param.Schema.SchemaProps
		}
	}

	path := swaggerPathToJinjaTemplate(basePath + functionPath)
	if len(queryParams) > 0 {
		path += "?" + strings.Join(queryParams, "&")
	}

	headersTemplate := map[string]string{":method": method}

	for _, name := range headerParams {
		headersTemplate[name] = fmt.Sprintf("{{default(%v, \"\")}}", name)
	}

	fnName := operation.ID
	if fnName == "" {
		fnName = strings.ToLower(method) + strings.Replace(functionPath, "/", ".", -1)
	}

	// build transformation:

	headerTemplatesForTransform := make(map[string]*transformation_plugins.InjaTemplate)

	needsTransformation := body != nil

	for k, v := range headersTemplate {
		needsTransformation = true
		headerTemplatesForTransform[k] = &transformation_plugins.InjaTemplate{Text: v}
	}

	if path != "" {
		needsTransformation = true
		headerTemplatesForTransform[":path"] = &transformation_plugins.InjaTemplate{Text: path}
	}

	if method == "GET" || method == "HEAD" {
		// this tells envoy to remove the body and content-type header completely
		headerTemplatesForTransform["content-type"] = &transformation_plugins.InjaTemplate{Text: ""}
		headerTemplatesForTransform["content-length"] = &transformation_plugins.InjaTemplate{Text: "0"}
		headerTemplatesForTransform["transfer-encoding"] = &transformation_plugins.InjaTemplate{Text: ""}
		clearBody := ""
		body = &clearBody
	} else {
		needsTransformation = true
		headerTemplatesForTransform["content-type"] = &transformation_plugins.InjaTemplate{Text: "application/json"}
	}

	// this function doesn't request any kind of transformation
	if !needsTransformation {
		return fnName, nil
	}
	transtemplate := &transformation_plugins.TransformationTemplate{
		Headers: headerTemplatesForTransform,
	}

	if method == "POST" || method == "PATCH" || method == "PUT" {
		transtemplate.BodyTransformation = &transformation_plugins.TransformationTemplate_Passthrough{
			Passthrough: &transformation_plugins.Passthrough{}}
	}

	if body != nil {
		transtemplate.BodyTransformation = &transformation_plugins.TransformationTemplate_Body{
			Body: &transformation_plugins.InjaTemplate{
				Text: *body,
			}}
	}

	return fnName, transtemplate
}

func getBodyTemplate(parent string, schema spec.SchemaProps, definitions spec.Definitions) string {
	bodyTemplate := "{"
	var fields []string
	for key, prop := range schema.Properties {
		var defaultValue string
		if prop.Default != nil {
			defaultValue = fmt.Sprintf("%v", prop.Default)
		}
		def := getDefinitionFor(prop.Ref, definitions)
		defaultValue = fmt.Sprintf("\"%v\"", defaultValue)
		paramName := "%v"
		if parent != "" {
			paramName = parent + ".%v"
		}
		switch {
		case def != nil:
			if def.Type.Contains("string") {
				fields = append(fields, fmt.Sprintf(`"%v": "{{ default(`+paramName+`, %v)}}"`, key, getBodyTemplate(parent+"."+key, def.SchemaProps, definitions), defaultValue))
			} else {
				fields = append(fields, fmt.Sprintf(`"%v": {{ default(`+paramName+`, %v) }}`, key, getBodyTemplate(parent+"."+key, def.SchemaProps, definitions), defaultValue))
			}
		case prop.Type.Contains("string"):
			// string needs escaping
			fields = append(fields, fmt.Sprintf(`"%v": "{{ default(`+paramName+`, %v)}}"`, key, key, defaultValue))
		default:
			fields = append(fields, fmt.Sprintf(`"%v": {{ default(`+paramName+`, %v) }}`, key, key, defaultValue))
		}
	}
	// idempotency
	sort.SliceStable(fields, func(i, j int) bool {
		return fields[i] < fields[j]
	})
	bodyTemplate += strings.Join(fields, ",")
	bodyTemplate += "}"
	return bodyTemplate
}

func getDefinitionFor(ref spec.Ref, definitions spec.Definitions) *spec.Schema {
	refName := strings.TrimPrefix(ref.String(), "#/definitions/")
	schema, ok := definitions[refName]
	if !ok {
		return nil
	}
	return &schema
}

func swaggerPathToJinjaTemplate(path string) string {
	path = strings.Replace(path, "{", "{{ default(", -1)
	path = strings.Replace(path, "}", ", \"\") }}", -1)
	return path
}
