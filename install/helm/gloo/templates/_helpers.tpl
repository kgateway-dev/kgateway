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
Reverse port order if $spec.service.httpsFirst is true.
*/}}
{{- define "spec.ports" -}}
{{- $spec := . -}}
{{- if $spec.service.httpsFirst | default false }}
  - port: {{ $spec.service.httpsPort }}
    targetPort: {{ $spec.podTemplate.httpsPort }}
    protocol: TCP
    name: https
  - port: {{ $spec.service.httpPort }}
    targetPort: {{ $spec.podTemplate.httpPort }}
    protocol: TCP
    name: http
{{- else }}  
  - port: {{ $spec.service.httpPort }}
    targetPort: {{ $spec.podTemplate.httpPort }}
    protocol: TCP
    name: http
  - port: {{ $spec.service.httpsPort }}
    targetPort: {{ $spec.podTemplate.httpsPort }}
    protocol: TCP
    name: https
{{- end -}}
{{- end -}}