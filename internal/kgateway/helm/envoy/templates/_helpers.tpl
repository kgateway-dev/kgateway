{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "kgateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Data-plane related macros:
*/}}


{{/*
Expand the name of the chart.
*/}}
{{- define "kgateway.gateway.name" -}}
{{- if .Values.gateway.name }}
{{- .Values.gateway.name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- default .Chart.Name .Values.gateway.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "kgateway.gateway.fullname" -}}
{{- if .Values.gateway.fullnameOverride }}
{{- .Values.gateway.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Release.Name .Values.gateway.nameOverride }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Constant labels - labels that are stable across releases
*/}}
{{- define "kgateway.gateway.constLabels" -}}
kgateway: kube-gateway
{{- end }}


{{/*
Common labels
*/}}
{{- define "kgateway.gateway.labels" -}}
{{ include "kgateway.gateway.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
gateway.networking.k8s.io/gateway-class-name: {{ .Values.gateway.gatewayClassName }}
app.kubernetes.io/managed-by: kgateway
{{- end }}

{{- define "kgateway.gateway.podLabels" -}}
{{ include "kgateway.gateway.selectorLabels" . }}
gateway.networking.k8s.io/gateway-class-name: {{ .Values.gateway.gatewayClassName }}
{{- end }}


{{/*
Selector labels
*/}}
{{- define "kgateway.gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kgateway.gateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
gateway.networking.k8s.io/gateway-name: {{ .Release.Name }}
{{- end }}

{{/*
All labels including selector labels, standard labels, and custom gateway labels
*/}}
{{- define "kgateway.gateway.allLabels" -}}
{{- $gateway := .Values.gateway -}}
{{- $labels := merge (dict
  "kgateway" "kube-gateway"
  "app.kubernetes.io/managed-by" "kgateway"
  "gateway.networking.k8s.io/gateway-class-name" .Values.gateway.gatewayClassName
  )
  (include "kgateway.gateway.selectorLabels" . | fromYaml)
  ($gateway.gatewayLabels | default dict)
-}}
{{- if .Chart.AppVersion -}}
{{- $_ := set $labels "app.kubernetes.io/version" .Chart.AppVersion -}}
{{- end -}}
{{- $labels | toYaml -}}
{{- end -}}

{{/*
Return a container image value as a string
*/}}
{{- define "kgateway.gateway.image" -}}
{{- if not .repository -}}
{{- fail "an Image's repository must be present" -}}
{{- end -}}
{{- $image := "" -}}
{{- if .registry -}}
{{- $image = printf "%s/%s" .registry .repository -}}
{{- else -}}
{{- $image = printf "%s" .repository -}}
{{- end -}}
{{- if .tag -}}
{{- $image = printf "%s:%s" $image .tag -}}
{{- end -}}
{{- if .digest -}}
{{- $image = printf "%s@%s" $image .digest -}}
{{- end -}}
{{ $image }}
{{- end -}}
