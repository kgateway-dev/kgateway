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
{{ .registry }}/{{ .repository }}{{ ternary "-fips" "" ( and (has .repository (list "gloo-ee" "extauth-ee" "gloo-ee-envoy-wrapper" "rate-limit-ee" )) (default false .fips)) }}:{{ .tag }}{{ ternary "-extended" "" (default false .extended) }}
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

gloo.util.merge is a fork of a helm library chart function (https://github.com/helm/charts/blob/master/incubator/common/templates/_util.tpl).
This includes some optimizations to speed up chart rendering time, and merges in a value (overrides) with a named template, unlike the upstream
version, which merges two named templates.

*/ -}}
{{- define "gloo.util.merge" -}}
{{- $top := first . -}}
{{- $overrides := (index . 1) -}}
{{- $tpl := fromYaml (include (index . 2) $top) -}}
{{- if or (empty $overrides) (empty $tpl) -}}
{{ include (index . 2) $top }} {{/* render source as is */}}
{{- else -}}
{{- $merged := mergeOverwrite $tpl $overrides -}}
{{- toYaml $merged -}} {{/* render source with overrides as YAML */}}
{{- end -}}
{{- end -}}

{{/*
Whether we need to wait for the validation service to be up and running before applying custom resources,
and whether we need to clean up custom resources on uninstall.
This is true if the validation webhook is enabled with a failurePolicy of Fail.

The input to this function should be the gloo helm values object.
*/}}
{{- define "gloo.customResourceLifecycle" -}}
{{- if and .gateway.enabled .gateway.validation.enabled .gateway.validation.webhook.enabled (eq .gateway.validation.failurePolicy "Fail") }}
true
{{- end }}{{/* if and .gateway.enabled .gateway.validation.enabled .gateway.validation.webhook.enabled (eq .gateway.validation.failurePolicy "Fail") */}}
{{- end -}}

{{/*
This snippet should be included under the metadata for any Gloo custom resources.

It is used to ensure that CRs that we validate are only installed after the validation service is running.
When the resource is applied as part of post-install/post-upgrade, we also need to explicitly add the helm
labels/annotations, since by default Helm does not manage hook resources and won't add the annotations.

The input to the function should be a dict with the following key/value mappings:
- "release": the helm release object
- "values": the gloo helm values object (this is provided as an argument so that other charts such as the
  GlooEE chart can also use this function and pass in the appropriate values from its subchart)
- "labels": (optional) additional labels to include (a dict of key/value pairs)
*/}}
{{- define "gloo.customResourceLabelsAndAnnotations" -}}
{{- $customResourceLifecycle := include "gloo.customResourceLifecycle" .values }}
  labels:
    app: gloo
{{- range $k, $v := .labels }}
    {{$k}}: {{$v}}
{{- end }}
{{- if $customResourceLifecycle }}
    created-by: gloo-install
    app.kubernetes.io/managed-by: Helm
  annotations:
    "meta.helm.sh/release-name": {{ .release.Name }}
    "meta.helm.sh/release-namespace": {{ .release.Namespace }}
    "helm.sh/hook": post-install,post-upgrade
    "helm.sh/hook-weight": "10" # must be installed after the gateway rollout job completes
{{- end -}}{{/* if $customResourceLifecycle */}}
{{- end -}}
