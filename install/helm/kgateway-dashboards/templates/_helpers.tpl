{{/*
Expand the name of the chart.
*/}}
{{- define "dashboards.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}
