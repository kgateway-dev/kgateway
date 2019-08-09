{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}


{{- define "gloo.role" -}}
{{- if .Values.rbac.namespaced -}}
Role
{{- else -}}
ClusterRole
{{- end -}}
{{- end -}}

{{- define "gloo.rolebindingsuffix" -}}
{{- if not .Values.rbac.namespaced -}}
-{{ .Release.Namespace }}
{{- end -}}
{{- end -}}
{{/*
Expand the name of a container image
*/}}
{{- define "gloo.image" -}}
{{ .registry }}/{{ .repository }}:{{ .tag }}
{{- end -}}
