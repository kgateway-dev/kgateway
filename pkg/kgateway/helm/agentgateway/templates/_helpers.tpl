{{/*
Expand the name of the chart.
*/}}
{{- define "kgateway.gateway.name" -}}
{{- .Values.agentgateway.name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "kgateway.gateway.fullname" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
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
{{- $gateway := .Values.agentgateway -}}
{{- $labels := merge (dict
  "kgateway" "kube-gateway"
  "app.kubernetes.io/managed-by" "kgateway"
  "gateway.networking.k8s.io/gateway-class-name" .Values.agentgateway.gatewayClassName
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
