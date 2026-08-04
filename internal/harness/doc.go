/*
Package harness implements capability detection and hook integration for Claude Code and Pi.

It provides the implementation of core.Harness and core.HarnessRegistry for Claude Code
and Pi. It reads, merges, and writes Claude Code's
config (~/.claude.json and ~/.claude/settings.json) to register the Dossier MCP server
and lifecycle hooks, detecting available capabilities (MCP, session-start/end and
pre-compaction hooks, transcript capture) so missing ones can be surfaced rather than
silently skipped.
*/
package harness
