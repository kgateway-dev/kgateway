{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}


{{- define "gloo.roleKind" -}}
{{- if .Values.global.glooRbac.namespaced -}}
Role
{{- else -}}
ClusterRole
{{- end -}}
{{- end -}}

{{- define "gloo.rbacNameSuffix" -}}
{{- if .Values.global.glooRbac.nameSuffix -}}
-{{ .Values.global.glooRbac.nameSuffix }}
{{- else if not .Values.global.glooRbac.namespaced -}}
-{{ .Release.Namespace }}
{{- end -}}
{{- end -}}

{{/*
Expand the name of a container image
*/}}
{{- define "gloo.image" -}}
{{ .registry }}/{{ .repository }}:{{ .tag }}
{{- end -}}

{{/*
Get the wasm version of the image.
1.2.3->1.2.3-wasm
1.2.3-rc1 -> 1.2.3-wasm-rc1
*/}}
{{- define "gloo.wasmImage" -}}
{{ .registry }}/{{ .repository }}:{{ regexReplaceAll "([0-9]+\\.[0-9]+\\.[0-9]+)" .tag "${1}-wasm" }}
{{- end -}}