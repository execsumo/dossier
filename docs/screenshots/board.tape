Output "/tmp/dossier-board.gif"
Set Shell "bash"
Set FontSize 18
Set Width 1600
Set Height 800
Set Theme "Catppuccin Mocha"
Set CursorBlink false
Hide
Type "export DOSSIER_HOME=$(cat /tmp/dossier-demo-home) PATH=$PWD:$PATH && clear" Enter
Type "dossier tui" Enter
Show
Sleep 3s
Type "v"
Sleep 3s
Screenshot docs/screenshots/tui-board.png
Sleep 1s
