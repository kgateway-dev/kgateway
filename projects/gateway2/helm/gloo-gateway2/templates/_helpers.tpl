{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "gloo-gateway2.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}


{{/*
Control-plane related macros:
*/}}


{{/*
Expand the name of the chart.
*/}}
{{- define "gloo-gateway2.controlPlane.name" -}}
{{- default .Chart.Name .Values.controlPlane.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "gloo-gateway2.controlPlane.fullname" -}}
{{- if .Values.controlPlane.fullnameOverride }}
{{- .Values.controlPlane.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.controlPlane.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | printf "%s-cp" | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s-cp" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "gloo-gateway2.controlPlane.labels" -}}
helm.sh/chart: {{ include "gloo-gateway2.chart" . }}
{{ include "gloo-gateway2.controlPlane.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "gloo-gateway2.controlPlane.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gloo-gateway2.controlPlane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "gloo-gateway2.controlPlane.serviceAccountName" -}}
{{- if .Values.controlPlane.serviceAccount.create }}
{{- default (include "gloo-gateway2.controlPlane.fullname" .) .Values.controlPlane.serviceAccount.name }}
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
{{- define "gloo-gateway2.gateway.name" -}}
{{- if .Values.gateway.name }}
{{- .Values.gateway.name | printf "%s-dp" | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- default .Chart.Name .Values.gateway.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "gloo-gateway2.gateway.fullname" -}}
{{- if .Values.gateway.fullnameOverride }}
{{- .Values.gateway.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else if .Values.gateway.name }}
{{- $name := default .Chart.Name .Values.gateway.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Values.gateway.name | printf "%s-dp" | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s-dp" .Release.Name .Values.gateway.name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- else }}
{{- $name := default .Chart.Name .Values.gateway.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | printf "%s-dp" | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s-dp" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "gloo-gateway2.gateway.labels" -}}
helm.sh/chart: {{ include "gloo-gateway2.chart" . }}
{{ include "gloo-gateway2.gateway.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "gloo-gateway2.gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gloo-gateway2.gateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "gloo-gateway2.gateway.serviceAccountName" -}}
{{- if .Values.gateway.serviceAccount.create }}
{{- default (include "gloo-gateway2.gateway.fullname" .) .Values.gateway.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.gateway.serviceAccount.name }}
{{- end }}
{{- end }}
