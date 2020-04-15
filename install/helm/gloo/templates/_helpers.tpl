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

{{/* Init container definition for envoy binary copy into gloo setup */}}
{{- define "gloo.copyenvoyinitcontainer" -}}
{{- $image := merge .Values.global.glooValidation.envoy.image .Values.global.image}}
- image: {{ template "gloo.image" $image }}
  imagePullPolicy: {{ $image.pullPolicy }}
  name: copy-envoy-binary
  volumeMounts:
    - name: envoy-binary-dir
      mountPath: /etc/bin/envoy
  command: ['sh', '-c', "cp /usr/local/bin/envoy /etc/bin/envoy"]
{{- end}}