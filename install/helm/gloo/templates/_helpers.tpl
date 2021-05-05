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
Expand the name of a container image
*/}}
{{- define "gloo.image" -}}
{{ .registry }}/{{ .repository }}:{{ .tag }}{{ ternary "-extended" "" (default false .extended) }}
{{- end -}}

{{- define "gloo.pullSecret" -}}
{{- if .pullSecret -}}
imagePullSecrets:
- name: {{ .pullSecret }}
{{- end -}}
{{- end -}}

{{- /*
gloo.util.merge will merge two YAML templates and output the result.

This takes an array of three values:
- the top context
- the template name of the overrides (destination)
- the template name of the base (source)

*/ -}}
{{- define "gloo.util.merge" -}}
{{- $top := first . -}}
{{- $overrides := fromYaml (index . 1) | default (dict ) -}}
{{- $tpl := fromYaml (include (index . 2) $top) | default (dict ) -}}
{{- $merged := merge $overrides $tpl -}}
{{- if not (empty $merged) -}}
{{- toYaml (merge $overrides $tpl) -}}
{{- end -}}
{{- end -}}

{{- define "gloo.util.safeAccessVar" -}}
{{- $top := first . -}}
{{- $string := (index . 1) -}}
{{- $matches := (splitList "." $string ) -}}
{{- $stop := false -}}
{{- range $index, $elem := $matches -}}
{{- if not $stop -}}
{{- if gt (len $elem) 0 }}
{{- $output := slice $matches 0 (add1 $index) | join "." -}}
{{- $test := (cat "{{ or (empty " $output ") (not (kindIs \"map\"" $output ")) }}") -}}
{{- $testRes := tpl $test $top }}
{{- if and (eq $testRes "true") (ne (add1 $index) (len $matches)) }}
{{- $stop = true -}}
{{ end -}}
{{ end -}}
{{ end -}}
{{ end -}}
{{- if $stop }}
{{- else }}
{{ tpl (cat "{{ toYaml " $string "}}") $top }}
{{- end }}
{{- end -}}
