{{- if . }}
{{- range . }}
{{- if (eq (len .Vulnerabilities) 0) }}
No Vulnerabilities Found for {{.Target}}
{{- else }}
Vulnerability ID|Package|Severity|Installed Version|Fixed Version|Reference
---|---|---|---|---|---
{{- range .Vulnerabilities }}
{{ .VulnerabilityID }}|{{ .PkgName }}|{{ .Vulnerability.Severity }}|{{ .InstalledVersion }}|{{ .FixedVersion }}|{{ .PrimaryURL }}
{{- end }}
{{- end }}
{{- end }}
{{- else }}
Trivy Returned Empty Report
{{- end }}
