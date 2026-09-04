package assets

import "embed"

// FS holds the embedded assets for Dossier (Distillation Guide, context templates,
// Claude Code skills, and the Pi integration extension).
//
//go:embed guide.md library.tmpl.md instructions.md dossier-delegate-skill.md spark-skill.md pi-extension.ts
var FS embed.FS
