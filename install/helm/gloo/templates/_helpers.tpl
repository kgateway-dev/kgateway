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

{{- define "gloo.rolebindingsuffix" -}}
{{- if not .Values.global.glooRbac.namespaced -}}
-{{ .Release.Namespace }}
{{- end -}}
{{- end -}}
{{/*
Expand the name of a container image
*/}}
{{- define "gloo.image" -}}
{{ .registry }}/{{ .repository }}:{{ .tag }}
{{- end -}}

{{/* This value makes its way into k8s labels, so if the implementation changes,
     make sure it's compatible with label values */}}
{{- define "gloo.installationId" -}}
{{- if not .Values.installConfig.installationId -}}
{{- $_ := set .Values.installConfig "installationId" (randAlpha 10) -}}
{{- end -}}
{{ .Values.installConfig.installationId }}
{{- end -}}
