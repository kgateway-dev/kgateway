{{/* vim: set filetype=mustache: */}}

{{- define "gloo.roleKind" -}}
{{- if .Values.global.glooRbac.namespaced -}}
Role
{{- else -}}
ClusterRole
{{- end -}}
{{- end -}}

{{- define "gloo.rbacNameSuffix" -}}
{{- if .Values.global.glooRbac.nameSuffix -}}
-{{ .Values.global.glooRbac.nameSuffix }}
{{- else if not .Values.global.glooRbac.namespaced -}}
-{{ .Release.Namespace }}
{{- end -}}
{{- end -}}

{{/*
Expand the name of a container image, adding -fips to the name of the repo if configured.
*/}}
{{- define "gloo.image" -}}
{{- if and .fips .fipsDigest -}}
{{- /*
In consideration of https://github.com/solo-io/gloo/issues/7326, we want the ability for -fips images to use their own digests,
rather than falling back (incorrectly) onto the digests of non-fips images
*/ -}}
{{ .registry }}/{{ .repository }}-fips:{{ .tag }}@{{ .fipsDigest }}
{{- else -}}
{{ .registry }}/{{ .repository }}{{ ternary "-fips" "" ( and (has .repository (list "gloo-ee" "extauth-ee" "gloo-ee-envoy-wrapper" "rate-limit-ee" )) (default false .fips)) }}:{{ .tag }}{{ ternary "-extended" "" (default false .extended) }}{{- if .digest -}}@{{ .digest }}{{- end -}}
{{- end -}}
{{- end -}}

{{- define "gloo.pullSecret" -}}
{{- if .pullSecret -}}
imagePullSecrets:
- name: {{ .pullSecret }}
{{ end -}}
{{- end -}}


{{- define "gloo.podSpecStandardFields" -}}
{{- with .nodeName -}}
nodeName: {{ . }}
{{ end -}}
{{- with .nodeSelector -}}
nodeSelector: {{ toYaml . | nindent 2 }}
{{ end -}}
{{- with .tolerations -}}
tolerations: {{ toYaml . | nindent 2 }}
{{ end -}}
{{- with .hostAliases -}}
hostAliases: {{ toYaml . | nindent 2 }}
{{ end -}}
{{- with .affinity -}}
affinity: {{ toYaml . | nindent 2 }}
{{ end -}}
{{- with .restartPolicy -}}
restartPolicy: {{ . }}
{{ end -}}
{{- with .priorityClassName -}}
priorityClassName: {{ . }}
{{ end -}}
{{- with .initContainers -}}
initContainers: {{ toYaml . | nindent 2 }}
{{ end -}}
{{- end -}}

{{- define "gloo.jobSpecStandardFields" -}}
{{- with .activeDeadlineSeconds -}}
activeDeadlineSeconds: {{ . }}
{{ end -}}
{{- with .backoffLimit -}}
backoffLimit: {{ . }}
{{ end -}}
{{- with .completions -}}
completions: {{ . }}
{{ end -}}
{{- with .manualSelector -}}
manualSelector: {{ . }}
{{ end -}}
{{- with .parallelism -}}
parallelism: {{ . }}
{{ end -}}
{{- /* include ttlSecondsAfterFinished if setTtlAfterFinished is undefined or equal to true.
      The 'kindIs' comparision is how we can check for undefined */ -}}
{{- if or (kindIs "invalid" .setTtlAfterFinished) .setTtlAfterFinished -}}
{{- with .ttlSecondsAfterFinished  -}}
ttlSecondsAfterFinished: {{ . }}
{{ end -}}
{{- end -}}
{{- end -}}


{{- define "gloo.containerSecurityContext" -}}
{{- $sec := (kindIs "invalid" .sec) | ternary dict .sec -}}
{{- $ctx := (kindIs "invalid" .ctx) | ternary dict .ctx -}}
{{- $fieldsToDisplay := or 
  (or (not (kindIs "invalid" $sec.allowPrivilegeEscalation)) (not (kindIs "invalid" $ctx.allowPrivilegeEscalation)) )
  (or .capabilities $ctx.useDefaultCapabilities)
  (not (kindIs "invalid" $sec.privileged))
  $sec.procMount
  (or (not (kindIs "invalid" $sec.readOnlyRootFilesystem)) (not (kindIs "invalid" $ctx.readOnlyRootFilesystem)) )
  $sec.runAsGroup 
  (or (not (kindIs "invalid" $sec.runAsNonRoot)) $ctx.runUnprivileged (not (kindIs "invalid" $ctx.runAsNonRoot)) )
  (and (not $ctx.floatingUserId) (or $sec.runAsUser $ctx.runAsUser))
  $sec.seLinuxOptions
  $sec.seccompProfile
  $sec.windowsOptions
 -}}
{{- if $fieldsToDisplay -}}
securityContext:
{{- if not (kindIs "invalid" $sec.allowPrivilegeEscalation) }}
  allowPrivilegeEscalation: {{ $sec.allowPrivilegeEscalation }}
{{- else if not (kindIs "invalid" $ctx.allowPrivilegeEscalation) }}
  allowPrivilegeEscalation: {{ $ctx.allowPrivilegeEscalation }}
{{- end }}
{{- if $sec.capabilities }}
  capabilities: {{ toYaml $sec.capabilities | nindent 4  }}
{{- else if $ctx.useDefaultCapabilities }}
  capabilities:
    drop:
    - ALL
    {{- if not $ctx.disableNetBind }}
    add:
    - NET_BIND_SERVICE
    {{- end}}
{{- end }}
{{- if not (kindIs "invalid" $sec.runAsNonRoot) }}
  runAsNonRoot: {{ $sec.runAsNonRoot }}
{{- else if not (kindIs "invalid" $ctx.runAsNonRoot) }}
  runAsNonRoot: {{ $ctx.runAsNonRoot }}
{{- else if $ctx.runUnprivileged }}
  runAsNonRoot: true
{{- end }}
{{- with $sec.procMount }}
  procMount: {{ . }}
{{- end }}
{{- if not (kindIs "invalid" $sec.readOnlyRootFilesystem) }}
  readOnlyRootFilesystem: {{ $sec.readOnlyRootFilesystem }}
{{- else if not (kindIs "invalid" $ctx.readOnlyRootFilesystem) }}
  readOnlyRootFilesystem: {{ $ctx.readOnlyRootFilesystem }}
{{- end }}
{{- with $sec.runAsGroup }}
  runAsGroup: {{ . }}
{{- end }}
{{- if not $ctx.floatingUserId }}
{{- with $sec.runAsUser }}
  runAsUser: {{ . }}
{{ end -}}
{{- if or $sec.runAsUser $ctx.runAsUser}}
  runAsUser: {{ or $sec.runAsUser $ctx.runAsUser  }}
{{- end }}
{{- end }}
{{- with $sec.seLinuxOptions }}
  seLinuxOptions: {{ toYaml . | nindent 4  }}
{{ end -}}
{{- with $sec.seccompProfile }}
  seccompProfile: {{ toYaml . | nindent 4 }}
{{ end -}}
{{- with $sec.windowsOptions }}
  windowsOptions: {{ toYaml . | nindent 4 }}
{{- end }}
{{- end }}
{{- end }}


{{- define "gloo.podSecurityContext" -}}
{{- $sec := (kindIs "invalid" .sec) | ternary dict .sec -}}
{{- $ctx := (kindIs "invalid" .ctx) | ternary dict .ctx -}}
{{- $fieldsToDisplay := or 
  $sec.fsGroupChangePolicy
  (or $sec.fsGroup $ctx.fsGroup)
  $sec.runAsGroup
  (or $sec.runAsUser $ctx.runAsUser)
  (not (kindIs "invalid" $ctx.runAsNonRoot) )
  (and $sec.runAsUser (not $ctx.floatingUserId))
  $sec.supplementalGroups
  $sec.seLinuxOptions 
  $sec.seccompProfile
  $sec.sysctls
  $sec.windowsOptions -}}
{{- if and $fieldsToDisplay (not $ctx.disablePodSecurityContext) -}}
securityContext:
{{- with $sec.fsGroupChangePolicy }}
  fsGroupChangePolicy: {{ . }}
{{- end }}
{{- if (or $sec.fsGroup $ctx.fsGroup) }}
  fsGroup: {{ printf "%.0f" (float64 (or $sec.fsGroup $ctx.fsGroup)) }}
{{- end }}
{{- with $sec.legacy }}
  legacy: {{ . }}
{{- end }}
{{- with $sec.runAsGroup }}
  runAsGroup: {{ . }}
{{- end }}
{{- if not (kindIs "invalid" $sec.runAsNonRoot) }}
  runAsNonRoot: {{ $sec.runAsNonRoot }}
{{- else if not (kindIs "invalid" $ctx.runAsNonRoot) }}
  runAsNonRoot: {{ $ctx.runAsNonRoot }}
{{- end }}
{{- if not $ctx.floatingUserId }}
{{- if (or $sec.runAsUser $ctx.runAsUser) }}
  runAsUser: {{ (or $sec.runAsUser $ctx.runAsUser) }}
{{- end }}
{{- end }}
{{- with $sec.supplementalGroups }}
  supplementalGroups: {{ . }}
{{- end }}
{{- with $sec.seLinuxOptions }}
  seLinuxOptions: {{ toYaml . | nindent 4 }}
{{- end }}
{{- with $sec.seccompProfile }}
  seccompProfile: {{ toYaml . | nindent 4 }}
{{- end }}
{{- with $sec.sysctls }}
  sysctls: {{ toYaml . | nindent 4 }}
{{- end }}
{{- with $sec.windowsOptions }}
  windowsOptions: {{ toYaml . | nindent 4 }}
{{- end }}
{{- end }}
{{- end }}

{{- /*
This takes an array of three values:
- the top context
- the yaml block that will be merged in (override)
- the name of the base template (source)

note: the source must be a named template (helm partial). This is necessary for the merging logic.

The behaviour is as follows, to align with already existing helm behaviour:
- If no source is found (template is empty), the merged output will be empty
- If no overrides are specified, the source is rendered as is
- If overrides are specified and source is not empty, overrides will be merged in to the source.

Overrides can replace / add to deeply nested dictionaries, but will completely replace lists.
Examples:

┌─────────────────────┬───────────────────────┬────────────────────────┐
│ Source (template)   │       Overrides       │        Result          │
├─────────────────────┼───────────────────────┼────────────────────────┤
│ metadata:           │ metadata:             │ metadata:              │
│   labels:           │   labels:             │   labels:              │
│     app: gloo       │    app: gloo1         │     app: gloo1         │
│     cluster: useast │    author: infra-team │     author: infra-team │
│                     │                       │     cluster: useast    │
├─────────────────────┼───────────────────────┼────────────────────────┤
│ lists:              │ lists:                │ lists:                 │
│   groceries:        │  groceries:           │   groceries:           │
│   - apple           │   - grapes            │   - grapes             │
│   - banana          │                       │                        │
└─────────────────────┴───────────────────────┴────────────────────────┘

gloo.util.merge is a fork of a helm library chart function (https://github.com/helm/charts/blob/master/incubator/common/templates/_util.tpl).
This includes some optimizations to speed up chart rendering time, and merges in a value (overrides) with a named template, unlike the upstream
version, which merges two named templates.

*/ -}}
{{- define "gloo.util.merge" -}}
{{- $top := first . -}}
{{- $overrides := (index . 1) -}}
{{- $tpl := fromYaml (include (index . 2) $top) -}}
{{- if or (empty $overrides) (empty $tpl) -}}
{{- include (index . 2) $top -}}{{/* render source as is */}}
{{- else -}}
{{- $merged := mergeOverwrite $tpl $overrides -}}
{{- toYaml $merged -}} {{/* render source with overrides as YAML */}}
{{- end -}}
{{- end -}}

{{/*
Returns the unique Gateway namespaces as defined by the helm values.
*/}}
{{- define "gloo.gatewayNamespaces" -}}
{{- $proxyNamespaces := list -}}
{{- range $key, $gatewaySpec := .Values.gatewayProxies -}}
  {{- $ns := $gatewaySpec.namespace | default $.Release.Namespace -}}
  {{- $proxyNamespaces = append $proxyNamespaces $ns -}}
{{- end -}}
{{- $proxyNamespaces = $proxyNamespaces | uniq -}}
{{ toJson $proxyNamespaces }}
{{- end -}}
