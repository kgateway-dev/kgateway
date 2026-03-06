{{/*
Expand the name of the chart.
*/}}
{{- define "kgateway.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "kgateway.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "kgateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kgateway.labels" -}}
helm.sh/chart: {{ include "kgateway.chart" . }}
{{ include "kgateway.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels | default dict }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kgateway.selectorLabels" -}}
kgateway: kgateway
app.kubernetes.io/name: {{ include "kgateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "kgateway.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kgateway.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Validate validation level and return the validated value.
Supported values: "standard" or "strict" (case-insensitive).
*/}}
{{- define "kgateway.validationLevel" -}}
{{- $level := .Values.validation.level | lower | trimAll " " -}}
{{- if or (eq $level "standard") (eq $level "strict") -}}
{{- $level -}}
{{- else -}}
{{- printf "ERROR: Invalid validation.level '%s'. Must be 'standard' or 'strict' (case-insensitive). Current value: '%s'" $level .Values.validation.level | fail -}}
{{- end -}}
{{- end }}

{{/*
Get the image tag with 'v' prefix for semver tags.
If the input already starts with 'v', return it as-is.
If the input looks like a semver version (e.g., "1.2.3"), prepend 'v'.
Otherwise (e.g., "latest", "dev"), return it unchanged.
*/}}
{{- define "kgateway.imageTag" -}}
{{- $tag := . -}}
{{- if hasPrefix "v" $tag -}}
{{- $tag -}}
{{- else if regexMatch "^[0-9]+\\.[0-9]+\\..*$" $tag -}}
{{- printf "v%s" $tag -}}
{{- else -}}
{{- $tag -}}
{{- end -}}
{{- end }}
