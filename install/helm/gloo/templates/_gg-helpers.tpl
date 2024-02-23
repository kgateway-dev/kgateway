{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "gloo-gateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}


{{/*
Control-plane related macros:
*/}}


{{/*
Expand the name of the chart.
*/}}
{{- define "gloo-gateway.controlPlane.name" -}}
{{- default .Chart.Name .Values.gateway2.controlPlane.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "gloo-gateway.controlPlane.fullname" -}}
{{- if .Values.gateway2.controlPlane.fullnameOverride }}
{{- .Values.gateway2.controlPlane.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Release.Name .Values.gateway2.controlPlane.nameOverride }}
{{- .Release.Name | printf "glood-%s" | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "gloo-gateway.controlPlane.labels" -}}
helm.sh/chart: {{ include "gloo-gateway.chart" . }}
{{ include "gloo-gateway.controlPlane.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "gloo-gateway.controlPlane.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gloo-gateway.controlPlane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Data-plane related macros:
*/}}

{{/*
Constant labels - labels that are stable across releases
We want this label to remain constant as it is used in glooctl version.
*/}}
{{- define "gloo-gateway.gateway.constLabels" -}}
gloo: gateway-v2
{{- end }}
