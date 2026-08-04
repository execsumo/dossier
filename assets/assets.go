package assets

import "embed"

// FS holds the embedded assets for Dossier (Distillation Guide, context templates,
// and the dossier-delegate Claude Code Skill).
//
//go:embed guide.md library.tmpl.md instructions.md dossier-delegate-skill.md
var FS embed.FS
