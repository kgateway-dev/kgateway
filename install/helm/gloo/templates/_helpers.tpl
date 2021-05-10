{{/* vim: set filetype=mustache: */}}

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
{{ .registry }}/{{ .repository }}:{{ .tag }}{{ ternary "-extended" "" (default false .extended) }}
{{- end -}}

{{- define "gloo.pullSecret" -}}
{{- if .pullSecret -}}
imagePullSecrets:
- name: {{ .pullSecret }}
{{ end -}}
{{- end -}}

{{- /*
gloo.util.merge will merge two YAML templates and output the result.

This takes an array of three values:
- the top context
- the template name of the overrides (destination)
- the template name of the base (source)

*/ -}}
{{- define "gloo.util.merge" -}}
{{- $top := first . -}}
{{- $overrides := (index . 1) -}}
{{- if empty $overrides -}}
{{ include (index . 2) $top}}
{{- else -}}
{{- $tpl := fromYaml (include (index . 2) $top) -}}
{{- if not (empty $tpl) -}}
{{- $merged := merge $overrides $tpl -}}
{{- if not (empty $merged) -}}
{{- toYaml (merge $overrides $tpl) -}}
{{- end -}}
{{- end -}}
{{- end -}} {{/*if not (empty $overrides)*/}}
{{- end -}}
