{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}

{{- define "gloo.testcontainer" -}}
- name: nginx
  image: nginx:1.7.9
  ports:
  - containerPort: 80
{{- end }}