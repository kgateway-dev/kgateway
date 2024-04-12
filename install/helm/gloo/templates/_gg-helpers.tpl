{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "gloo-gateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Expand the name of the chart.
*/}}
{{- define "gloo-gateway.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "gloo-gateway.fullname" -}}
{{- .Release.Name | printf "glood-%s" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "gloo-gateway.labels" -}}
helm.sh/chart: {{ include "gloo-gateway.chart" . }}
app.kubernetes.io/name: {{ include "gloo-gateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Constant labels - labels that are stable across releases
We want this label to remain constant as it is used in glooctl version.
*/}}
{{- define "gloo-gateway.constLabels" -}}
gloo: kube-gateway
{{- end }}
