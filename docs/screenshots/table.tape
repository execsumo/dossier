Output /tmp/dossier-table.gif
Set Shell "bash"
Set FontSize 20
Set Theme "Catppuccin Mocha"
Set CursorBlink false
Set TypingSpeed 0
Hide
Type "export DOSSIER_HOME=$(cat /tmp/dossier-demo-home) PATH=$PWD:$PATH && clear" Enter
Type "dossier tui" Enter
Show
Sleep 3s
Screenshot docs/screenshots/tui-table.png
Sleep 1s
