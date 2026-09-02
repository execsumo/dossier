# Dossier Library

{{if .Warnings}}
Warnings:
{{range .Warnings}}- {{.}}
{{end}}
{{end}}

The following dossiers are available to resume work on:
{{if .OpenDossiers}}
{{range .OpenDossiers}}- **{{.Name}}** (status: {{.Status}}, priority: {{.Priority}}, slug: {{.Slug}})
  {{if .Description}}Description: {{.Description}}
  {{end}}Next Action: {{.NextAction}}
{{end}}
{{else}}
(No open dossiers found)
{{end}}

Distillation Guide:
See: ~/.dossier/context/guide.md
