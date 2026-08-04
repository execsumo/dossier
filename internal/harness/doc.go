/*
Package harness implements capability detection and integration installation for
Claude Code and Pi, providing core.Harness and core.HarnessRegistry.

For Claude Code it reads, merges, and writes the user's config (~/.claude.json
and ~/.claude/settings.json) to register the Dossier MCP server and lifecycle
hooks. For Pi it installs the bundled Dossier Pi extension, which supplies the
session identity Pi does not otherwise expose outside its bash tool; pisession.go
owns that pointer record and the process-ancestry walk that finds it.

Capabilities are reported as they actually are — a capability Dossier does not
provide for a harness (Pi's lifecycle hooks, Pi's MCP) is reported unavailable so
it is surfaced rather than silently skipped. session.go holds the session-id
resolution ladder shared by the CLI and MCP adapters (ADR 0003, ADR 0005).
*/
package harness
