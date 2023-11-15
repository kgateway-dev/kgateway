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
{{- default .Chart.Name .Values.controlPlane.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "gloo-gateway.controlPlane.fullname" -}}
{{- if .Values.controlPlane.fullnameOverride }}
{{- .Values.controlPlane.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Release.Name .Values.controlPlane.nameOverride }}
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
Create the name of the service account to use
*/}}
{{- define "gloo-gateway.controlPlane.serviceAccountName" -}}
{{- if .Values.controlPlane.serviceAccount.create }}
{{- default (include "gloo-gateway.controlPlane.fullname" .) .Values.controlPlane.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.controlPlane.serviceAccount.name }}
{{- end }}
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
{{- define "gloo-gateway.gateway.const_labels" -}}
gloo: gateway-v2
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
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "gloo-gateway.gateway.serviceAccountName" -}}
{{- if .Values.gateway.serviceAccount.create }}
{{- default (include "gloo-gateway.gateway.fullname" .) .Values.gateway.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.gateway.serviceAccount.name }}
{{- end }}
{{- end }}
