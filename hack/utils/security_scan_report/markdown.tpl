{{- if . }}
{{- range . }}
{{- if (eq (len .Vulnerabilities) 0) }}
No Vulnerabilities Found for {{.Target}}
{{- else }}
Package|Vulnerability ID|Severity|Installed Version|Fixed Version
---|---|---|---|---
{{- range .Vulnerabilities }}
{{ .PkgName }}|{{ .VulnerabilityID }}|{{ .Vulnerability.Severity }}|{{ .InstalledVersion }}|{{ .FixedVersion }}
{{- end }}
{{- end }}
{{- end }}
{{- else }}
Trivy Returned Empty Report
{{- end }}
