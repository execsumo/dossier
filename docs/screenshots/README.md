# README screenshots

Regenerate with [VHS](https://github.com/charmbracelet/vhs) (needs `ttyd`, `ffmpeg`; set `VHS_NO_SANDBOX=true` on Linux as root-less CI):

```bash
go build -o dossier ./cmd/dossier
H=$(mktemp -d); echo "$H" > /tmp/dossier-demo-home
DOSSIER_BIN=./dossier docs/screenshots/seed.sh "$H"
vhs docs/screenshots/table.tape && vhs docs/screenshots/board.tape
```

Never run `dossier init` against the demo store: it edits the real harness config even with `--home`.
