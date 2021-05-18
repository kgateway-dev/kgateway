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


{{- define "gloo.podSpecStandardFields" -}}
{{- with .nodeName -}}
nodeName: {{ . }}
{{ end -}}
{{- with .nodeSelector -}}
nodeSelector: {{ toYaml . | nindent 2 }}
{{ end -}}
{{- with .tolerations -}}
tolerations: {{ toYaml . | nindent 2 }}
{{ end -}}
{{- with .hostAliases -}}
hostAliases: {{ toYaml . | nindent 2 }}
{{ end -}}
{{- with .affinity -}}
affinity: {{ toYaml . | nindent 2 }}
{{ end -}}
{{- with .restartPolicy -}}
restartPolicy: {{ . }}
{{ end -}}
{{- end -}}