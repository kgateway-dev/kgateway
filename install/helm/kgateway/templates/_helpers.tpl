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
app.kubernetes.io/component: controller
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

{{/*
Convert a Kubernetes memory quantity to bytes for GOMEMLIMIT percentage calculation.
Supported suffixes: Ki, Mi, Gi, Ti, Pi, Ei, k, M, G, T, P, E, or no suffix.
*/}}
{{- define "kgateway.memToBytes" -}}
{{- $memory := printf "%v" . -}}
{{- $number := $memory -}}
{{- $multiplier := 1.0 -}}
{{- if hasSuffix "Ki" $memory -}}
  {{- $number = trimSuffix "Ki" $memory -}}
  {{- $multiplier = 1024.0 -}}
{{- else if hasSuffix "Mi" $memory -}}
  {{- $number = trimSuffix "Mi" $memory -}}
  {{- $multiplier = 1048576.0 -}}
{{- else if hasSuffix "Gi" $memory -}}
  {{- $number = trimSuffix "Gi" $memory -}}
  {{- $multiplier = 1073741824.0 -}}
{{- else if hasSuffix "Ti" $memory -}}
  {{- $number = trimSuffix "Ti" $memory -}}
  {{- $multiplier = 1099511627776.0 -}}
{{- else if hasSuffix "Pi" $memory -}}
  {{- $number = trimSuffix "Pi" $memory -}}
  {{- $multiplier = 1125899906842624.0 -}}
{{- else if hasSuffix "Ei" $memory -}}
  {{- $number = trimSuffix "Ei" $memory -}}
  {{- $multiplier = 1152921504606846976.0 -}}
{{- else if hasSuffix "k" $memory -}}
  {{- $number = trimSuffix "k" $memory -}}
  {{- $multiplier = 1000.0 -}}
{{- else if hasSuffix "M" $memory -}}
  {{- $number = trimSuffix "M" $memory -}}
  {{- $multiplier = 1000000.0 -}}
{{- else if hasSuffix "G" $memory -}}
  {{- $number = trimSuffix "G" $memory -}}
  {{- $multiplier = 1000000000.0 -}}
{{- else if hasSuffix "T" $memory -}}
  {{- $number = trimSuffix "T" $memory -}}
  {{- $multiplier = 1000000000000.0 -}}
{{- else if hasSuffix "P" $memory -}}
  {{- $number = trimSuffix "P" $memory -}}
  {{- $multiplier = 1000000000000000.0 -}}
{{- else if hasSuffix "E" $memory -}}
  {{- $number = trimSuffix "E" $memory -}}
  {{- $multiplier = 1000000000000000000.0 -}}
{{- end -}}
{{- if not (regexMatch "^([0-9]+(\\.[0-9]+)?|\\.[0-9]+)([eE][+-]?[0-9]+)?$" $number) -}}
  {{- fail (printf "goMemLimitPercent: unsupported memory quantity %q; use a Kubernetes memory quantity such as \"2Gi\", \"1.5Gi\", or \"512M\"" $memory) -}}
{{- end -}}
{{- printf "%.0f" (floor (mulf (float64 $number) $multiplier)) -}}
{{- end -}}

{{/* Compute a byte-valued GOMEMLIMIT from a memory quantity and percentage. */}}
{{- define "kgateway.goMemLimit" -}}
{{- $memory := index . 0 -}}
{{- $percent := int (index . 1) -}}
{{- if or (lt $percent 1) (gt $percent 100) -}}
  {{- fail (printf "goMemLimitPercent must be between 1 and 100, got %d" $percent) -}}
{{- end -}}
{{- $bytes := include "kgateway.memToBytes" $memory | trim | float64 -}}
{{- printf "%.0f" (floor (divf (mulf $bytes (float64 $percent)) 100.0)) -}}
{{- end -}}
