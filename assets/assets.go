package assets

import "embed"

// FS holds the embedded assets for Dossier (Distillation Guide, context templates,
// the dossier-delegate Claude Code Skill, and the Pi session-identity extension).
//
//go:embed guide.md library.tmpl.md instructions.md dossier-delegate-skill.md pi-extension.ts
var FS embed.FS
