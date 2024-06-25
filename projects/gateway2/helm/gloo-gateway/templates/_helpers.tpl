{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "gloo-gateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Data-plane related macros:
*/}}


{{/*
Expand the name of the chart.
*/}}
{{- define "gloo-gateway.gateway.name" -}}
{{- if .Values.gateway.name }}
{{- .Values.gateway.name | printf "gloo-proxy-%s" | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- default .Chart.Name .Values.gateway.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "gloo-gateway.gateway.fullname" -}}
{{- if .Values.gateway.fullnameOverride }}
{{- .Values.gateway.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Release.Name .Values.gateway.nameOverride }}
{{- .Release.Name | printf "gloo-proxy-%s" | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Constant labels - labels that are stable across releases
We want this label to remain constant as it is used in glooctl version.
*/}}
{{- define "gloo-gateway.gateway.constLabels" -}}
gloo: kube-gateway
{{- end }}


{{/*
Common labels
*/}}
{{- define "gloo-gateway.gateway.labels" -}}
helm.sh/chart: {{ include "gloo-gateway.chart" . }}
{{ include "gloo-gateway.gateway.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "gloo-gateway.gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gloo-gateway.gateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
gateway.networking.k8s.io/gateway-name: {{ .Release.Name }}
{{- end }}

{{/*
Return a container image value as a string
*/}}
{{- define "gloo-gateway.gateway.image" -}}
{{- $image := printf "%s/%s:%s" .registry .repository .tag -}}
{{- if .digest -}}
{{- $image = printf "%s@%s" $image .digest -}}
{{- end -}}{{- /* if .digest */ -}}
{{ $image }}
{{- end -}}{{- /* define "gloo-gateway.gateway.image" */ -}}
