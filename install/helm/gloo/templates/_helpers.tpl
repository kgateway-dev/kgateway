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


{{- define "gloo.jobHelmDeletePolicySucceeded" -}}
{{- /* include a hook delete policy unless setTtlAfterFinished is either undefined or true and
      ttlSecondsAfterFinished is set. The 'kindIs' comparision is how we can check for
      undefined */ -}}
{{- if not (and .ttlSecondsAfterFinished (or (kindIs "invalid" .setTtlAfterFinished) .setTtlAfterFinished)) -}}
"helm.sh/hook-delete-policy": hook-succeeded
{{ end -}}
{{ end -}}

{{- define "gloo.jobHelmDeletePolicySucceededAndBeforeCreation" -}}
{{- /* include hook delete policy based on whether setTtlAfterFinished is undefined or equal to
      true. If it is the case, only delete explicitly before hook creation. Otherwise, also
      delete also on success. The 'kindIs' comparision is how we can check for undefined */ -}}
{{- if or (kindIs "invalid" .setTtlAfterFinished) .setTtlAfterFinished -}}
"helm.sh/hook-delete-policy": before-hook-creation
{{- else -}}
"helm.sh/hook-delete-policy": hook-succeeded,before-hook-creation
{{ end -}}
{{ end -}}

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

{{- /* 
This template is used to generate the gloo pod or container security context.
It takes 2 values:
  .values - the securityContext passed from the user in values.yaml
  .defaults - the default securityContext for the pod or container

  Depending upon the value of .values.merge, the securityContext will be merged with the defaults or completely replaced.
  In a merge, the values in .values will override the defaults, following the logic of helm's merge function.
Because of this, if a value is "true" in defaults it can not be modified with this method.
*/ -}}
{{- define "gloo.securityContext" }}
{{- $securityContext := dict -}}
{{- $overwrite := true -}}
{{- if .values -}}
  {{- if .values.mergePolicy }}
    {{- if eq .values.mergePolicy "helm-merge" -}}
      {{- $overwrite = false -}}
    {{- else if ne .values.mergePolicy "no-merge" -}}
      {{- fail printf "value '%s' is not an allowed value for mergePolicy. Allowed values are 'no-merge', 'helm-merge', or an empty string" .values.mergePolicy }}
    {{- end -}}
  {{- end }}
{{- end -}}

{{- if $overwrite -}}
  {{- $securityContext = or .values .defaults (dict) -}}
{{- else -}}
  {{- $securityContext = merge .values .defaults }}
{{- end }}
{{- /* Remove "mergePolicy" if it exists because it is not a part of the kubernetes securityContext definition */ -}}
{{- $securityContext = omit $securityContext "mergePolicy" -}}
{{- with $securityContext -}}
securityContext:{{ toYaml . | nindent 2 }}
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

gloo.util.merge is a fork of a helm library chart function (https://github.com/helm/charts/blob/main/incubator/common/templates/_util.tpl).
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


{{/* 
Generated the "operations" array for a resource for the ValidatingWebhookConfiguration
Arguments are a resource name, and a list of resources for which to skip webhook validation for DELETEs
This list is expected to come from `gateway.validation.webhook.skipDeleteValidationResources`
If the resource is in the list, or the list contains "*", it will generate ["Create", "Update"]
Otherwise it will generate ["Create", "Update", "Delete"]
*/}}
{{- define "gloo.webhookvalidation.operationsForResource" -}}
{{- $resource := first . -}}
{{- $skip := or (index . 1) list -}}
{{- $operations := list "CREATE" "UPDATE" -}}
{{- if not (or (has $resource $skip) (has "*" $skip)) -}}
  {{- $operations = append $operations "DELETE" -}}
{{- end -}}
{{ toJson  $operations -}}
{{- end -}}
