{{/* vim: set filetype=mustache: */}}

{{- /*
There can be cases when we do not want to overwrite an empty value on a resource when merged.
Eg. To generate a proxy config, we mergeOverwrite it with the default gateway-proxy config.
If we want to preserve the empty value of the gateway and not have them overwritten, we set it to `gloo.omitOverwrite`
and call `gloo.util.mergeOverwriteWithOmit` when merging. This sets all fields with values equal to this back to empty after the overwrite
*/ -}}
{{- define "gloo.omitOverwrite" }}
{{ printf "\n" }}{{/* This template is set to a new line. There may be scenarios where a field is initailly set to this value and the same field is appended to later on. Since this is just a new line, it won't cause rendering issues */}}
{{ end -}}
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
Construct a container image name from a registry, repository, tag, and digest.
*/}}
{{- define "gloo.image" -}}
{{- $image := printf "%s/%s" .registry .repository -}}

{{- /*
for fips or fips-distroless variants: add -fips to the image repo (name)
*/ -}}
{{- if or .fips (has .variant (list "fips" "fips-distroless")) -}}
{{- $fipsSupportedImages := list "gloo-ee" "extauth-ee" "gloo-ee-envoy-wrapper" "rate-limit-ee" "discovery-ee" "sds-ee" -}}
{{- if (has .repository $fipsSupportedImages) -}}
{{- $image = printf "%s-fips" $image -}}
{{- end -}}{{- /* if (has .repository $fipsSupportedImages) */ -}}
{{- end -}}{{- /* if or .fips (has .variant (list "fips" "fips-distroless")) */ -}}

{{- /*
add tag, if it exists
*/ -}}
{{- if .tag -}}
{{- $image = printf "%s:%s" $image .tag -}}
{{- end -}}{{- /* if .tag */ -}}

{{- /*
for distroless or fips-distroless variants: add -distroless to the tag
*/ -}}
{{- if and .tag (has .variant (list "distroless" "fips-distroless")) -}}
{{- $distrolessSupportedImages := list "gloo" "gloo-envoy-wrapper" "discovery" "sds" "certgen" "kubectl" "access-logger" "ingress" "gloo-ee" "extauth-ee" "gloo-ee-envoy-wrapper" "rate-limit-ee" "discovery-ee" "sds-ee" "observability-ee" "caching-ee" -}}
{{- if (has .repository $distrolessSupportedImages) -}}
{{- $image = printf "%s-distroless" $image -}} {{- /* Add distroless suffix to the tag since it contains the same binaries in a different container */ -}}
{{- end -}}{{- /* if (has .repository $distrolessSupportedImages) */ -}}
{{- end -}}{{- /* if and .tag (has .variant (list "distroless" "fips-distroless")) */ -}}

{{- /*
add digest for the chosen variant, if it exists
*/ -}}
{{- if or .fips (eq .variant "fips") -}}
  {{- if .fipsDigest -}}
    {{- $image = printf "%s@%s" $image .fipsDigest -}}
  {{- end -}}{{- /* if .fipsDigest */ -}}
{{- else if eq .variant "distroless" -}}
  {{- if .distrolessDigest -}}
    {{- $image = printf "%s@%s" $image .distrolessDigest -}}
  {{- end -}}{{- /* if .distrolessDigest */ -}}
{{- else if eq .variant "fips-distroless" -}}
  {{- if .fipsDistrolessDigest -}}
    {{- $image = printf "%s@%s" $image .fipsDistrolessDigest -}}
  {{- end -}}{{- /* if .fipsDistrolessDigest */ -}}
{{- else -}}
  {{- if .digest -}}{{- /* standard image digest */ -}}
    {{- $image = printf "%s@%s" $image .digest -}}
  {{- end -}}{{- /* if .digest */ -}}
{{- end -}}
{{ $image }}
{{- end -}}{{- /* define "gloo.image" */ -}}


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
{{- if and .ttlSecondsAfterFinished (or (kindIs "invalid" .setTtlAfterFinished) .setTtlAfterFinished) -}}
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
It takes 4 values:
  .values - the securityContext passed from the user in values.yaml
  .defaults - the default securityContext for the pod or container
  .globalSec - global security settings, usually from .Values.global.securitySettings
  .indent - the number of spaces to indent the output. If not set, the output will not be indented.
    The indentation argument is necessary because it is possible that no output will be rendered. 
    If that happens and the caller handles the indentation the result will be a line of whitespace, which gets caught by the whitespace tests

  Depending upon the value of .values.merge, the securityContext will be merged with the defaults or completely replaced.
  In a merge, the values in .values will override the defaults, following the logic of helm's merge function.
Because of this, if a value is "true" in defaults it can not be modified with this method.
*/ -}}
{{- define "gloo.securityContext" }}
{{- /* Move input parameters to non-null variables */ -}}
{{- $defaults := dict -}}
{{- if .defaults -}}
  {{- $defaults = .defaults -}}
{{- end -}}
{{- $values := dict -}}
{{- if .values -}}
  {{- $values = .values -}}
{{- end -}}
{{- $globalSec := dict -}}
{{- if .globalSec -}}
  {{- $globalSec = .globalSec -}}
{{- end -}}
{{ $indent := 0}}
{{- if .indent -}}
  {{- $indent = .indent -}}
{{- end -}}
{{- /* create $overwrite and set it based on the merge-policy */ -}}
{{- $overwrite := true -}}
{{- if $values.mergePolicy }}
  {{- if eq $values.mergePolicy "helm-merge" -}}
    {{- $overwrite = false -}}
  {{- else if ne $values.mergePolicy "no-merge" -}}
    {{- fail printf "value '%s' is not an allowed value for mergePolicy. Allowed values are 'no-merge', 'helm-merge', or an empty string" $values.mergePolicy }}
  {{- end -}}
{{- end -}}
{{- /* create $securityContext and combine with $defaults based on teh value of $overwrite */ -}}
{{- $securityContext := dict -}}
{{- if $overwrite -}}
  {{- $securityContext = or $values $defaults (dict) -}}
{{- else -}}
  {{- $securityContext = merge $values $defaults -}}
{{- end }}
{{- /* Apply global overrides */ -}}
{{- with $globalSec -}}
  {{- if .floatingUserId -}}
    {{- $_ := unset $securityContext "runAsUser" -}}
  {{- end -}}
{{- end -}}
{{- /* Remove "mergePolicy" if it exists because it is not a part of the kubernetes securityContext definition */ -}}
{{- $securityContext = omit $securityContext "mergePolicy" -}}
{{- with $securityContext -}}
  {{- $toRender := dict "securityContext" $securityContext -}}
  {{- toYaml $toRender | nindent $indent -}}
{{- end }}
{{- end }}


{{- /*
This template is used to generate the container security context.
It takes 4 values:
  .values - the securityContext passed from the user in values.yaml
  .defaults - the default securityContext for the pod or container
  .podSecurityStandards - podSecurityStandard from values.yaml
  .globalSec - global security settings, usually from .Values.global.securitySettings
  .indent - the number of spaces to indent the output. If not set, the output will not be indented.
    The indentation argument is necessary because it is possible that no output will be rendered. 
    If that happens and the caller handles the indentation the result will be a line of whitespace, which gets caught by the whitespace tests

  If .podSecurityStandards.container.enableRestrictedContainerDefaults is true, the defaults will be set to a restricted set of values.
  .podSecurityStandards.container.defaultSeccompProfileType can be used to set the seccompProfileType.
*/ -}}
{{- define "gloo.containerSecurityContext" -}}
{{- /* Move input parameters to non-null variables */ -}}
{{- $defaults := dict -}}
{{- if .defaults -}}
  {{- $defaults = .defaults -}}
{{- end -}}
{{- $values := dict -}}
{{- if .values -}}
  {{- $values = .values -}}
{{- end -}}
{{ $indent := 0}}
{{- if .indent -}}
  {{- $indent = .indent -}}
{{- end -}}
{{ $pss := dict }}
{{- if .podSecurityStandards -}}
  {{- $pss = .podSecurityStandards -}}
{{- end -}}
{{- /* set default seccompProfileType */ -}}

{{- $pss_restricted_defaults := dict 
    "runAsNonRoot" true
    "capabilities" (dict "drop" (list "ALL"))
    "allowPrivilegeEscalation" false }}
{{- /* set defaults if appropriate */ -}}
{{- if $pss.container -}}
  {{/* Set the default seccompProfileType */}}
  {{- $defaultSeccompProfileType := "RuntimeDefault"}}
  {{- if $pss.container.defaultSeccompProfileType -}}
    {{- $defaultSeccompProfileType = $pss.container.defaultSeccompProfileType -}}
    {{- if and (ne $defaultSeccompProfileType "RuntimeDefault") (ne $defaultSeccompProfileType "Localhost") -}}
      {{- fail printf "value '%s' is not an allowed value for defaultSeccompProfileType. Allowed values are 'RuntimeDefault' or 'Localhost'" . }}
    {{- end -}}
  {{- end -}}
  {{- $_ := set $pss_restricted_defaults  "seccompProfile" (dict "type" $defaultSeccompProfileType) -}}
  {{- if $pss.container.enableRestrictedContainerDefaults -}}
    {{- $defaults = merge $defaults $pss_restricted_defaults -}}
  {{- end -}}
{{- end -}}
{{- /* call general securityContext template */ -}}
{{- include "gloo.securityContext" (dict 
            "values" $values
            "defaults" $defaults
            "indent" $indent
            "globalSec" .globalSec) -}}
{{- end -}}


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

{{- define "gloo.util.mergeOverwriteWithOmit" -}}
{{- $resource := first . -}}
{{- $overwrite := index . 1 -}}
{{- $result := deepCopy $resource | mergeOverwrite (deepCopy $overwrite) -}}
{{- range $key, $value := $result }}
  {{- if eq (toString $value) "gloo.omitOverwrite" -}}
    {{- $_ := unset $result $key }}
  {{- end -}}
{{- end -}}
{{ toJson $result }}
{{- end -}}

{{/* Additional labels added to every resource */}}
{{- define "gloo.labels" -}}
app: gloo
{{- with .Values.global.additionalLabels | default dict }}
{{ toYaml . }}
{{- end }}
{{- end }}
