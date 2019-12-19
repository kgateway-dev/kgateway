{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}


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
{{ .registry }}/{{ .repository }}:{{ .tag }}
{{- end -}}

{{/*
Expects its context to be a struct of the form { Deployment: {stats: bool}, GlobalContext: .Values.global }
Returns a STRING of "true" if 1. the deployment explicitly wants stats, or 2. stats are enabled globally and the deployment has not disabled stats
Returns "false" otherwise
*/}}
{{- define "gloo.statsServerEnabled" -}}

{{/* NOTE: Go templating doesn't do short circuiting in its conditional logic. Thus the weird structure here */}}
{{- if .Deployment.stats -}}
{{- if .Deployment.stats.enabled -}}
true
{{- else -}}
false
{{- end -}} {{/* end if deployment.stats.enabled */}}
{{- else if .GlobalContext.glooStats.enabled -}}
true
{{- else -}}
false
{{- end -}}
{{- end -}}
