{{- define "defaultGateway.gateway" -}}
{{- $name := (index . 1) }}
{{- $spec := (index . 2) }}
{{- with (first .) }}
{{- $gatewaySettings := $spec.gatewaySettings }}
{{- if $gatewaySettings.enabled }}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: {{ $name | kebabcase }}
  namespace: {{ $spec.namespace | default .Release.Namespace }}
  labels:
    app: gloo
spec:
  {{- if $gatewaySettings.ipv4Only }}
  bindAddress: "0.0.0.0"
  {{- else }}
  bindAddress: "::"
  {{- end }}
  bindPort: {{ $spec.podTemplate.httpPort }}
{{- if $gatewaySettings.httpHybridGateway }}
{{ toYaml $gatewaySettings.httpHybridGateway | indent 2}}
{{- end }}
{{- if $gatewaySettings.customHttpGateway}}
  httpGateway:
{{ toYaml $gatewaySettings.customHttpGateway | indent 4}}
{{- else if $spec.tracing }}
{{- if $spec.tracing.provider }}
  httpGateway:
    options:
      httpConnectionManagerSettings:
        tracing:
{{ toYaml $spec.tracing.provider | indent 10 }}
{{- end }}
{{- else }}
  httpGateway: {}
{{- end }}
{{- if or ($gatewaySettings.options) ($gatewaySettings.accessLoggingService) }}
  options:
{{- end }}
  {{- if $gatewaySettings.options }}
  {{ toYaml $gatewaySettings.options | nindent 4 }}
  {{- end }}
  {{- if $gatewaySettings.accessLoggingService }}
    accessLoggingService:
  {{- toYaml $gatewaySettings.accessLoggingService | nindent 6 }}
  {{- end }}
  useProxyProto: {{ $gatewaySettings.useProxyProto }}
  ssl: false
  proxyNames:
  - {{ $name | kebabcase }}
{{- end }}{{/* $gatewaySettings.enabled */}}
{{- end }}{{/* with */}}
{{- end }}{{/* define "defaultGateway.gateway" */}}


{{- define "defaultGateway.sslGateway" -}}
{{- $name := (index . 1) }}
{{- $spec := (index . 2) }}
{{- with (first .) }}
{{- $gatewaySettings := $spec.gatewaySettings }}
{{- if $gatewaySettings.enabled }}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: {{ $name | kebabcase }}-ssl
  namespace: {{ $spec.namespace | default .Release.Namespace }}
  labels:
    app: gloo
spec:
  {{- if $gatewaySettings.ipv4Only }}
  bindAddress: "0.0.0.0"
  {{- else }}
  bindAddress: "::"
  {{- end }}
  bindPort: {{ $spec.podTemplate.httpsPort }}
{{- if $gatewaySettings.httpsHybridGateway }}
{{ toYaml $gatewaySettings.httpsHybridGateway | indent 2}}
{{- end }}
{{- if $gatewaySettings.customHttpsGateway }}
  httpGateway:
{{ toYaml $gatewaySettings.customHttpsGateway | indent 4}}
{{- else if $spec.tracing }}
{{- if $spec.tracing.provider }}
  httpGateway:
    options:
      httpConnectionManagerSettings:
        tracing:
{{ toYaml $spec.tracing.provider | indent 10 }}
{{- end }}{{/* if $spec.tracing.provider */}}
{{- else }}
  httpGateway: {}
{{- end }}{{/* if $gatewaySettings.customHttpsGateway */}}
{{- if or ($gatewaySettings.options) ($gatewaySettings.accessLoggingService) }}
  options:
{{- end }}
  {{- if $gatewaySettings.options }}
  {{ toYaml $gatewaySettings.options | nindent 4 }}
  {{- end }}
  {{- if $gatewaySettings.accessLoggingService }}
    accessLoggingService:
  {{- toYaml $gatewaySettings.accessLoggingService | nindent 6 }}
  {{- end }}
  useProxyProto: {{ $gatewaySettings.useProxyProto }}
  ssl: true
  proxyNames:
  - {{ $name | kebabcase }}
{{- end }}{{/* $gatewaySettings.enabled */}}
{{- end }}{{/* with */}}
{{- end }}{{/* define "defaultGatway.sslGateway" */}}

{{- define "defaultGateway.failoverGateway" -}}
{{- $name := (index . 1) }}
{{- $spec := (index . 2) }}
{{- with (first .) }}
{{- $gatewaySettings := $spec.gatewaySettings }}
{{- if $gatewaySettings.enabled }}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: {{ $name | kebabcase }}-failover
  namespace: {{ $spec.namespace | default .Release.Namespace }}
  labels:
    app: gloo
spec:
{{- if $gatewaySettings.ipv4Only }}
  bindAddress: "0.0.0.0"
{{- else }}
  bindAddress: "::"
{{- end }}
  bindPort: {{ $spec.failover.port }}
  tcpGateway:
    tcpHosts:
    - name: failover
      sslConfig:
        secretRef:
          name: {{ $spec.failover.secretName }}
          namespace: {{ .Release.Namespace }}
      destination:
        forwardSniClusterName: {}
  proxyNames:
  - {{ $name | kebabcase }}
{{- end }}{{/* $gatewaySettings.enabled */}}
{{- end }}{{/* with */}}
{{- end }}{{/* define "defaultGateway.failoverGateway" */}}

{{- define "gloo.customResources.defaultGateways" -}}
{{- $gatewayProxy := .Values.gatewayProxies.gatewayProxy }}
{{- range $name, $gatewaySpec := .Values.gatewayProxies }}
{{- $spec := deepCopy $gatewaySpec | mergeOverwrite (deepCopy $gatewayProxy) }}
{{- $gatewaySettings := $spec.gatewaySettings }}
{{- if and $spec.gatewaySettings (not $gatewaySpec.disabled) }}
{{- $ctx := (list $ $name $spec) }}
{{- if not $gatewaySettings.disableGeneratedGateways }}
{{- if not $gatewaySettings.disableHttpGateway }}
{{- $defaultGatewayOverride := $spec.gatewaySettings.httpGatewayKubeOverride }}
---
{{ include "gloo.util.merge" (list $ctx $defaultGatewayOverride "defaultGateway.gateway") -}}
{{- end }}{{/* if not $gatewaySettings.disableHttpGateway */}}
{{- if not $gatewaySettings.disableHttpsGateway }}
{{- $sslGatewayOverride := $spec.gatewaySettings.httpsGatewayKubeOverride }}
---
{{ include "gloo.util.merge" (list $ctx $sslGatewayOverride "defaultGateway.sslGateway") -}}
{{- end }}{{/* if not $gatewaySettings.disableHttpsGateway  */}}
{{- end }}{{/* if not $gatewaySettings.disableGeneratedGateways */}}
{{- if $spec.failover }}
{{- if $spec.failover.enabled }}
{{- $failoverGatewayOverride := $spec.failover.kubeResourceOverride }}
---
{{ include "gloo.util.merge" (list $ctx $failoverGatewayOverride "defaultGateway.failoverGateway") -}}
{{- end }}{{/* if $spec.failover.enabled */}}
{{- end }}{{/* if $spec.failover */}}
{{- end }}{{/* if $spec.gatewaySettings and (not $spec.disabled) */}}
{{- end }}{{/* range gateways */}}
{{- end }}{{/* define "gloo.customResources.defaultGateways" */}}
