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

{{/* This value makes its way into k8s labels, so if the implementation changes,
     make sure it's compatible with label values */}}
{{- define "gloo.installationId" -}}
{{- if not .Values.global.glooInstallationId -}}
{{- $_ := set .Values.global "glooInstallationId" (randAlphaNum 20) -}}
{{- end -}}
{{ .Values.global.glooInstallationId }}
{{- end -}}
