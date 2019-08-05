{{/*
Expand the name of the chart.
*/}}
{{- define "gloo.image" -}}
{{- if .registry }}
{{ .registry }}/{{ .repository }}:{{ .tag }}
{{- else}}
{{ .repository }}:{{ .tag }}
{{- end }}
{{- end -}}
