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

{{/*
Images valid for the GatewayParameters
ref Image api in projects/gateway2/api/v1alpha1/kube/container.proto
*/}}
{{- define "gloo-gateway.gatewayParametersImage" -}}
{{- $image := . -}}
{{- if $image.registry }}
registry: {{ $image.registry }}
{{- end -}}{{/* if $image.registry */}}
{{- if $image.repository }}
repository: {{ $image.repository }}
{{- end -}}{{/* if $image.repository */}}
{{- if $image.tag }}
tag: {{ $image.tag }}
{{- end -}}{{/* if $image.tag */}}
{{- if $image.digest }}
digest: {{ $image.digest }}
{{- end -}}{{/* if $image.digest */}}
{{- if $image.pullPolicy }}
pullPolicy: {{ $image.pullPolicy }}
{{- end -}}{{/* if $image.pullPolicy */}}
{{- end }}
