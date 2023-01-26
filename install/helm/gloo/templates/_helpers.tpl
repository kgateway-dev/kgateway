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
{{- $sec := or .sec dict -}}
{{- $ctx := or .ctx dict -}}
{{- $fieldsToDisplay := or 
  (or (not (kindIs "invalid" $sec.allowPrivilegeEscalation)) (not (kindIs "invalid" $ctx.allowPrivilegeEscalation)) )
  (or $sec.capabilities $ctx.capabilities)
  (or (not (kindIs "invalid" $sec.privileged)) (not (kindIs "invalid" $ctx.privileged)))
  (or $sec.procMount $ctx.procMount)
  (or (not (kindIs "invalid" $sec.readOnlyRootFilesystem)) (not (kindIs "invalid" $ctx.readOnlyRootFilesystem)) )
  (or $sec.runAsGroup $ctx.runAsGroup )
  (or (not (kindIs "invalid" $sec.runAsNonRoot)) $ctx.runUnprivileged)
  (and (not $ctx.floatingUserId) (or $sec.runAsUser $ctx.runAsUser))
  (or $sec.seLinuxOptions $ctx.seLinuxOptions)
  (or $sec.seccompProfile $ctx.seccompProfile)
  (or $sec.windowsOptions $ctx.windowsOptions)
 -}}
{{- if $fieldsToDisplay -}}
securityContext:
{{- if not (kindIs "invalid" $sec.allowPrivilegeEscalation) }}
  allowPrivilegeEscalation: {{ $sec.allowPrivilegeEscalation }}
{{- else if not (kindIs "invalid" $ctx.allowPrivilegeEscalation) }}
  allowPrivilegeEscalation: {{ $ctx.allowPrivilegeEscalation }}
{{- end }}
{{- if or $sec.capabilities $ctx.capabilities }}
  capabilities: {{- toYaml (or $sec.capabilities $ctx.capabilities) | nindent 4  }}
{{- end }}
{{- if not (kindIs "invalid" $sec.runAsNonRoot) }}
  runAsNonRoot: {{ $sec.runAsNonRoot }}
{{- else if not (kindIs "invalid" $ctx.runAsNonRoot) }}
  runAsNonRoot: {{ $ctx.runAsNonRoot }}
{{- else if $ctx.runUnprivileged }}
  runAsNonRoot: true
{{- end }}
{{- if or $sec.procMount $ctx.procMount  }}
  procMount: {{ (or $sec.procMount $ctx.procMount) }}
{{- end }}
{{- if not (kindIs "invalid" $sec.readOnlyRootFilesystem) }}
  readOnlyRootFilesystem: {{ $sec.readOnlyRootFilesystem }}
{{- else if not (kindIs "invalid" $ctx.readOnlyRootFilesystem) }}
  readOnlyRootFilesystem: {{ $ctx.readOnlyRootFilesystem }}
{{- end }}
{{- if or $sec.runAsGroup $ctx.runAsGroup  }}
  runAsGroup: {{ (or $sec.runAsGroup $ctx.runAsGroup) }}
{{- end }}
{{- if or $sec.runAsUser $ctx.runAsUser}}
  runAsUser: {{ or $sec.runAsUser $ctx.runAsUser  }}
{{- end }}
{{- if or $sec.seLinuxOptions $ctx.seLinuxOptions }}
  seLinuxOptions: {{- toYaml (or $sec.seLinuxOptions $ctx.seLinuxOptions) | nindent 4  }}
{{- end }}
{{- if or $sec.seccompProfile $ctx.seccompProfile }}
  seccompProfile: {{- toYaml (or $sec.seccompProfile $ctx.seccompProfile) | nindent 4  }}
{{- end }}
{{- if or $sec.windowsOptions $ctx.windowsOptions }}
  windowsOptions: {{- toYaml (or $sec.windowsOptions $ctx.windowsOptions) | nindent 4  }}
{{- end }}
{{- end }}
{{- end }}


{{- define "gloo.podSecurityContext" -}}
{{- $sec := or .sec dict -}}
{{- $ctx := or .ctx dict -}}
{{- $fieldsToDisplay := or 
  (or $sec.fsGroupChangePolicy $ctx.fsGroupChangePolicy)
  (or $sec.fsGroup $ctx.fsGroup)
  (or $sec.runAsGroup $ctx.runAsGroup)
  (or $sec.runAsUser $ctx.runAsUser)
  (or (not (kindIs "invalid" $sec.runAsNonRoot) ) (not (kindIs "invalid" $ctx.runAsNonRoot) ))
  (or $sec.supplementalGroups $ctx.supplementalGroups)
  (or $sec.seLinuxOptions $ctx.seLinuxOptions)
  (or $sec.seccompProfile $ctx.seccompProfile)
  (or $sec.sysctls $ctx.sysctls)
  (or $sec.windowsOptions $ctx.windowsOptions)
 -}}
{{- if and $fieldsToDisplay (not $ctx.disablePodSecurityContext) -}}
securityContext:
{{- if (or $sec.fsGroupChangePolicy $ctx.fsGroupChangePolicy) }}
  fsGroupChangePolicy: {{ (or $sec.fsGroupChangePolicy $ctx.fsGroupChangePolicy) }}
{{- end }}
{{- if (or $sec.fsGroup $ctx.fsGroup) }}
  fsGroup: {{ printf "%.0f" (float64 (or $sec.fsGroup $ctx.fsGroup)) }}
{{- end }}
{{- if (or $sec.legacy $ctx.legacy) }}
  legacy: {{ (or $sec.legacy $ctx.legacy) }}
{{- end }}
{{- if (or $sec.runAsGroup $ctx.runAsGroup) }}
  runAsGroup: {{ (or $sec.runAsGroup $ctx.runAsGroup) }}
{{- end }}
{{- if not (kindIs "invalid" $sec.runAsNonRoot) }}
  runAsNonRoot: {{ $sec.runAsNonRoot }}
{{- else if not (kindIs "invalid" $ctx.runAsNonRoot) }}
  runAsNonRoot: {{ $ctx.runAsNonRoot }}
{{- end }}
{{- if (or $sec.runAsUser $ctx.runAsUser) }}
  runAsUser: {{ (or $sec.runAsUser $ctx.runAsUser) }}
{{- end }}
{{- if (or $sec.supplementalGroups $ctx.supplementalGroups) }}
  supplementalGroups: {{ (or $sec.supplementalGroups $ctx.supplementalGroups) }}
{{- end }}
{{- if or $sec.seLinuxOptions $ctx.seLinuxOptions }}
  seLinuxOptions: {{- toYaml (or $sec.seLinuxOptions $ctx.seLinuxOptions) | nindent 4  }}
{{- end }}
{{- if or $sec.seccompProfile $ctx.seccompProfile }}
  seccompProfile: {{- toYaml (or $sec.seccompProfile $ctx.seccompProfile) | nindent 4  }}
{{- end }}
{{- if or $sec.windowsOptions $ctx.windowsOptions }}
  windowsOptions: {{- toYaml (or $sec.windowsOptions $ctx.windowsOptions) | nindent 4  }}
{{- end }}
{{- if or $sec.windowsOptions $ctx.sysctls }}
  sysctls: {{- toYaml (or $sec.sysctls $ctx.sysctls) | nindent 4  }}
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
