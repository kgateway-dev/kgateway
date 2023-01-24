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
{{- $fieldsToDisplay := or 
  (not (kindIs "invalid" .allowPrivilegeEscalation))
  .capabilities
  (not (kindIs "invalid" .privileged))
  .procMount
  (not (kindIs "invalid" .readOnlyRootFilesystem))
  .runAsGroup 
  (not (kindIs "invalid" .runAsNonRoot))
  (and (not .floatingUserId) .runAsUser)
  .seLinuxOptions
  .seccompProfile
  .windowsOptions
 -}}
securityContext:
{{- if not (kindIs "invalid" .allowPrivilegeEscalation) }}
  allowPrivilegeEscalation: {{ .allowPrivilegeEscalation }}
{{- end }}
{{- with .capabilities }}
  capabilities: {{ toYaml . | nindent 4  }}
{{- end }}
{{- if not (kindIs "invalid" .privileged) }}
  privileged: {{ .privileged }}
{{- end }}
{{- with .procMount }}
  procMount: {{ . }}
{{- end }}
{{- if not (kindIs "invalid" .readOnlyRootFilesystem) }}
  readOnlyRootFilesystem: {{ .readOnlyRootFilesystem }}
{{- end }}
{{- with .runAsGroup }}
  runAsGroup: {{ . }}
{{- end }}
{{- if not (kindIs "invalid" .runAsNonRoot) }}
  runAsNonRoot: {{ .runAsNonRoot }}
{{- end }}
{{- if not .floatingUserId }}
{{- with .runAsUser }}
  runAsUser: {{ . }}
{{ end -}}
{{ end -}}
{{- with .seLinuxOptions }}
  seLinuxOptions: {{ toYaml . | nindent 4  }}
{{ end -}}
{{- with .seccompProfile }}
  seccompProfile: {{ toYaml . | nindent 4 }}
{{ end -}}
{{- with .windowsOptions }}
  windowsOptions: {{ toYaml . | nindent 4 }}
{{- end }}
{{- end }}


{{- define "gloo.containerSecurityContext2" -}}
{{- $fieldsToDisplay := or 
  (not (kindIs "invalid" .conf.allowPrivilegeEscalation))
  .capabilities
  (not (kindIs "invalid" .conf.privileged))
  .conf.procMount
  (not (kindIs "invalid" .conf.readOnlyRootFilesystem))
  .conf.runAsGroup 
  (or (not (kindIs "invalid" .conf.runAsNonRoot)) .ctx.runUnprivileged)
  (and (not .ctx.floatingUserId) (or .conf.runAsUser .ctx.runAsUser))
  .conf.seLinuxOptions
  .conf.seccompProfile
  .conf.windowsOptions
 -}}
{{- if $fieldsToDisplay -}}
securityContext:
{{- if not (kindIs "invalid" .conf.allowPrivilegeEscalation) }}
  allowPrivilegeEscalation: {{ .conf.allowPrivilegeEscalation }}
{{- end }}
{{- if .conf.capabilities }}
  capabilities: {{ toYaml .conf.capabilities | nindent 4  }}
{{- else if .ctx.useDefaultCapabilities }}
  capabilities:
    drop:
    - ALL
    {{- if not .ctx.disableNetBind }}
    add:
    - NET_BIND_SERVICE
    {{- end}}
{{- end }}
{{- if not (kindIs "invalid" .conf.runAsNonRoot) }}
  runAsNonRoot: {{ .conf.runAsNonRoot }}
{{- else if .ctx.runUnprivileged }}
  runAsNonRoot: true
{{- end }}
{{- with .conf.procMount }}
  procMount: {{ . }}
{{- end }}
{{- if not (kindIs "invalid" .conf.readOnlyRootFilesystem) }}
  readOnlyRootFilesystem: {{ .conf.readOnlyRootFilesystem }}
{{- end }}
{{- with .conf.runAsGroup }}
  runAsGroup: {{ . }}
{{- end }}

{{- if not .ctx.floatingUserId }}
{{- with .conf.runAsUser }}
  runAsUser: {{ . }}
{{ end -}}
{{ end -}}
{{- with .conf.seLinuxOptions }}
  seLinuxOptions: {{ toYaml . | nindent 4  }}
{{ end -}}
{{- with .conf.seccompProfile }}
  seccompProfile: {{ toYaml . | nindent 4 }}
{{ end -}}
{{- with .conf.windowsOptions }}
  windowsOptions: {{ toYaml . | nindent 4 }}
{{- end }}
{{- end }}
{{- end }}


{{- define "gloo.podSecurityContext2" -}}
{{- $fieldsToDisplay := or 
  .conf.fsGroupChangePolicy
  (or .conf.fsGroup .ctx.fsGroup)
  .conf.runAsGroup
  (or .conf.runAsUser .ctx.runAsUser)
  (not (kindIs "invalid" .ctx.runAsNonRoot) )
  (and .conf.runAsUser (not .ctx.floatingUserId))
  .conf.supplementalGroups
  .conf.seLinuxOptions 
  .conf.seccompProfile
  .conf.sysctls
  .conf.windowsOptions -}}
{{- if and $fieldsToDisplay .ctx.enablePodSecurityContext -}}
securityContext:
{{- with .conf.fsGroupChangePolicy }}
  fsGroupChangePolicy: {{ . }}
{{- end }}
{{- if (or .conf.fsGroup .ctx.fsGroup) }}
  fsGroup: {{ printf "%.0f" (float64 (or .conf.fsGroup .ctx.fsGroup)) }}
{{- end }}
{{- with .conf.legacy }}
  legacy: {{ . }}
{{- end }}
{{- with .conf.runAsGroup }}
  runAsGroup: {{ . }}
{{- end }}
{{- if not (kindIs "invalid" .conf.runAsNonRoot) }}
  runAsNonRoot: {{ .conf.runAsNonRoot }}
{{- end }}
{{- if not .ctx.floatingUserId }}
{{- if (or .conf.runAsUser .ctx.runAsUser) }}
  runAsUser: {{ (or .conf.runAsUser .ctx.runAsUser) }}
{{- end }}
{{- end }}
{{- with .conf.supplementalGroups }}
  supplementalGroups: {{ . }}
{{- end }}
{{- with .conf.seLinuxOptions }}
  seLinuxOptions: {{ toYaml . | nindent 4 }}
{{- end }}
{{- with .conf.seccompProfile }}
  seccompProfile: {{ toYaml . | nindent 4 }}
{{- end }}
{{- with .conf.sysctls }}
  sysctls: {{ toYaml . | nindent 4 }}
{{- end }}
{{- with .conf.windowsOptions }}
  windowsOptions: {{ toYaml . | nindent 4 }}
{{- end }}
{{- end }}
{{- end }}


{{- define "gloo.podSecurityContext" -}}
{{- $fieldsToDisplay := or 
  .fsGroupChangePolicy
  .fsGroup
  .legacy
  .runAsGroup
  (not (kindIs "invalid" .runAsNonRoot) )
  (and .runAsUser (not .floatingUserId))
  .supplementalGroups
  .seLinuxOptions 
  .seccompProfile
  .sysctls
  .windowsOptions -}}
{{- if and $fieldsToDisplay .enablePodSecurityContext -}}
securityContext:
{{- with .fsGroupChangePolicy }}
  fsGroupChangePolicy: {{ . }}
{{- end }}
{{- with .fsGroup }}
  fsGroup: {{ printf "%.0f" (float64 .) }}
{{- end }}
{{- with .legacy }}
  legacy: {{ . }}
{{- end }}
{{- with .runAsGroup }}
  runAsGroup: {{ . }}
{{- end }}
{{- if not (kindIs "invalid" .runAsNonRoot) }}
  runAsNonRoot: {{ .runAsNonRoot }}
{{- end }}
{{- if not .floatingUserId }}
{{- with .runAsUser }}
  runAsUser: {{ . }}
{{- end }}
{{- end }}
{{- with .supplementalGroups }}
  supplementalGroups: {{ . }}
{{- end }}
{{- with .seLinuxOptions }}
  seLinuxOptions: {{ toYaml . | nindent 4 }}
{{- end }}
{{- with .seccompProfile }}
  seccompProfile: {{ toYaml . | nindent 4 }}
{{- end }}
{{- with .sysctls }}
  sysctls: {{ toYaml . | nindent 4 }}
{{- end }}
{{- with .windowsOptions }}
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
