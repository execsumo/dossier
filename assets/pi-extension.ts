/**
 * Dossier — Pi session identity bridge.
 *
 * Installed by `dossier init` / `dossier harness install pi`. Do not hand-edit:
 * Dossier rewrites this file (backing up whatever it replaces) whenever the
 * bundled version changes.
 *
 * Why it exists
 * -------------
 * Pi exports PI_SESSION_ID / PI_SESSION_FILE only into the *bash tool's* spawn
 * environment. Any Dossier process Pi did not spawn through the bash tool — an
 * MCP server started by an MCP adapter extension, a long-lived helper — never
 * sees them, and a process that was spawned once keeps its snapshot of the
 * environment even after the user runs /new, /resume, or /fork. Without a
 * session id Dossier refuses to bind a Dossier (no global active Dossier;
 * binding is per session), so the agent cannot switch topics from inside Pi.
 *
 * What it does
 * ------------
 * On every session start (startup, new, resume, fork, reload) it:
 *   1. writes a pointer file keyed by the *Pi process id* recording the live
 *      session id and session file, and
 *   2. mirrors both into this process's environment, so every child Pi spawns
 *      from that point on inherits the identity for free.
 * Dossier resolves a session id by walking its own process ancestry until it
 * finds the pointer belonging to the Pi process that owns it, which keeps
 * concurrent Pi sessions isolated from each other.
 *
 * Scope
 * -----
 * Session identity only. Bridging Pi's lifecycle into `dossier hook
 * session-start|session-end|pre-compaction` is deliberately not wired here
 * yet; see docs/harness-capabilities.md.
 */

import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";

/** Bumped only when the pointer record's shape changes incompatibly. */
const POINTER_SCHEMA = 1;

interface SessionPointer {
	schema: number;
	pid: number;
	session_id: string;
	session_file?: string;
	cwd?: string;
	reason?: string;
	updated_at: string;
}

function expandTilde(target: string): string {
	if (target === "~") return os.homedir();
	if (target.startsWith("~/")) return path.join(os.homedir(), target.slice(2));
	return target;
}

/**
 * Pointer directory. Kept inside Pi's own agent directory so both sides can
 * derive it from the environment alone, and so it disappears with Pi.
 * DOSSIER_PI_SESSION_DIR overrides it (Dossier reads the same variable).
 */
function pointerDir(): string {
	const override = process.env.DOSSIER_PI_SESSION_DIR;
	if (override) return expandTilde(override);
	const agentDir = process.env.PI_CODING_AGENT_DIR ?? path.join(os.homedir(), ".pi", "agent");
	return path.join(expandTilde(agentDir), "dossier", "sessions");
}

function pointerPath(pid: number): string {
	return path.join(pointerDir(), `${pid}.json`);
}

function isAlive(pid: number): boolean {
	try {
		process.kill(pid, 0);
		return true;
	} catch (err) {
		// EPERM means the process exists but belongs to another user.
		return (err as NodeJS.ErrnoException).code === "EPERM";
	}
}

/** Drop pointers left behind by Pi processes that are gone (crash, SIGKILL). */
function prunePointers(): void {
	const dir = pointerDir();
	let entries: string[];
	try {
		entries = fs.readdirSync(dir);
	} catch {
		return;
	}
	for (const entry of entries) {
		const match = /^(\d+)\.json$/.exec(entry);
		if (!match) continue;
		const pid = Number(match[1]);
		if (pid === process.pid || isAlive(pid)) continue;
		try {
			fs.rmSync(path.join(dir, entry));
		} catch {
			// A pointer we cannot remove is not worth failing a session over.
		}
	}
}

function publish(ctx: ExtensionContext, reason: string): SessionPointer {
	const sessionId = ctx.sessionManager.getSessionId();
	if (!sessionId) {
		throw new Error("Pi reported no session id");
	}
	const sessionFile = ctx.sessionManager.getSessionFile();

	// Mirror into this process's environment: children Pi spawns from here on
	// (MCP servers, pi.exec, other extensions) inherit the identity directly and
	// never need the pointer file.
	process.env.PI_SESSION_ID = sessionId;
	if (sessionFile) {
		process.env.PI_SESSION_FILE = sessionFile;
	} else {
		delete process.env.PI_SESSION_FILE;
	}

	const pointer: SessionPointer = {
		schema: POINTER_SCHEMA,
		pid: process.pid,
		session_id: sessionId,
		session_file: sessionFile,
		cwd: ctx.cwd,
		reason,
		updated_at: new Date().toISOString(),
	};

	const dir = pointerDir();
	fs.mkdirSync(dir, { recursive: true });
	const target = pointerPath(process.pid);
	// Write-then-rename: a reader never observes a half-written pointer.
	const tmp = `${target}.${process.pid}.tmp`;
	fs.writeFileSync(tmp, `${JSON.stringify(pointer, null, 2)}\n`, { mode: 0o600 });
	fs.renameSync(tmp, target);
	return pointer;
}

function clearPointer(): void {
	try {
		fs.rmSync(pointerPath(process.pid));
	} catch {
		// Already gone, or never written.
	}
}

export default function (pi: ExtensionAPI) {
	let published: SessionPointer | undefined;
	let lastError: string | undefined;

	pi.on("session_start", async (event, ctx) => {
		prunePointers();
		try {
			published = publish(ctx, event.reason);
			lastError = undefined;
		} catch (err) {
			published = undefined;
			lastError = err instanceof Error ? err.message : String(err);
			// Degrade visibly: a silent failure here looks like Dossier simply
			// forgetting which topic the session was on.
			ctx.ui.notify(
				`Dossier: could not publish this Pi session's identity (${lastError}). ` +
					`Dossier tools will report no session until this is fixed.`,
				"warning",
			);
		}
	});

	pi.on("session_shutdown", async (event) => {
		// "new" / "resume" / "fork" are followed by a session_start that
		// overwrites the pointer; only a real quit leaves it dangling.
		if (event.reason === "quit") {
			clearPointer();
			published = undefined;
		}
	});

	pi.registerCommand("dossier-session", {
		description: "Show the Pi session identity Dossier will resolve",
		handler: async (_args, ctx) => {
			const lines: string[] = [];
			const sessionId = published?.session_id ?? ctx.sessionManager.getSessionId();
			lines.push(`session id: ${sessionId ?? "unavailable"}`);
			lines.push(`session file: ${ctx.sessionManager.getSessionFile() ?? "none (ephemeral session)"}`);
			lines.push(`pointer: ${published ? pointerPath(process.pid) : "not published"}`);
			if (lastError) lines.push(`last error: ${lastError}`);
			ctx.ui.notify(`Dossier\n${lines.join("\n")}`, lastError ? "warning" : "info");
		},
	});
}
