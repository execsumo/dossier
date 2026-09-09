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
 * Lifecycle bridging
 * ------------------
 * Pi has extension *events*, not out-of-process hooks, so this extension is
 * also what turns them into `dossier hook session-start|session-end|
 * pre-compaction` invocations:
 *   - session_start      -> `session-start`, whose stdout is injected into the
 *                           session as a custom message (deliverAs "nextTurn",
 *                           so it lands in context without triggering a turn).
 *   - session_shutdown   -> `session-end`, before the pointer is cleared.
 *   - session_before_compact -> `pre-compaction`, returning nothing so
 *                           compaction itself is never cancelled or rewritten.
 * The child process inherits PI_SESSION_ID/PI_SESSION_FILE from the mirror
 * below, so the CLI resolves this session's identity and transcript on its own
 * and no payload has to be piped in.
 *
 * Scope
 * -----
 * Session identity, lifecycle bridging, and the `/spark` capture command. MCP
 * is deliberately not wired: Pi ships no MCP client, and Dossier's Pi surface
 * is the CLI. See docs/harness-capabilities.md.
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
	hostname?: string;
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
		hostname: os.hostname(),
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

/** Lifecycle events Dossier bridges, named as the CLI subcommands they invoke. */
type HookEvent = "session-start" | "session-end" | "pre-compaction";

/**
 * Budget for one hook. Generous for session-start (it reads the store and may
 * inline a Distilled State) but bounded, because a hung Dossier must never be
 * able to wedge Pi's startup or block a quit.
 */
const HOOK_TIMEOUT_MS: Record<HookEvent, number> = {
	"session-start": 15_000,
	"session-end": 10_000,
	"pre-compaction": 10_000,
};

/**
 * The Dossier binary to invoke. DOSSIER_BIN wins so a non-PATH install still
 * works; otherwise PATH resolution matches what the user types themselves.
 *
 * Deliberately not templated in at install time: PiExtensionInstalled compares
 * this file byte-for-byte against the embedded asset to decide whether the
 * integration is current, and a machine-specific path in the source would make
 * that comparison always fail and rewrite the file on every init.
 */
function dossierBin(): string {
	return process.env.DOSSIER_BIN?.trim() || "dossier";
}

export default function (pi: ExtensionAPI) {
	let published: SessionPointer | undefined;
	let lastError: string | undefined;
	const hookErrors = new Map<HookEvent, string>();

	/**
	 * Run one lifecycle hook and return its stdout, or undefined when it did not
	 * run. Never throws: a failing hook degrades this session's durable memory,
	 * which is worth a warning, but must not take Pi's session down with it.
	 */
	const runHook = async (event: HookEvent, ctx: ExtensionContext): Promise<string | undefined> => {
		if (!published) {
			// Without a published identity the CLI would resolve no session (or,
			// worse, a stale one) and file the result against the wrong Dossier.
			return undefined;
		}
		try {
			const result = await pi.exec(dossierBin(), ["hook", event], {
				cwd: ctx.cwd,
				timeout: HOOK_TIMEOUT_MS[event],
			});
			if (result.code !== 0) {
				const detail = result.stderr.trim() || result.stdout.trim() || `exit ${result.code}`;
				noteHookFailure(event, ctx, result.killed ? `timed out after ${HOOK_TIMEOUT_MS[event]}ms` : detail);
				return undefined;
			}
			hookErrors.delete(event);
			return result.stdout;
		} catch (err) {
			noteHookFailure(event, ctx, err instanceof Error ? err.message : String(err));
			return undefined;
		}
	};

	/**
	 * Surface a hook failure once per distinct message per session. Repeating an
	 * identical warning on every compaction would train the user to ignore it.
	 */
	const noteHookFailure = (event: HookEvent, ctx: ExtensionContext, detail: string): void => {
		if (hookErrors.get(event) === detail) return;
		hookErrors.set(event, detail);
		const consequence: Record<HookEvent, string> = {
			"session-start": "this session started without its Dossier context",
			"session-end": "this session's transcript was not archived",
			"pre-compaction": "state was not saved before compaction",
		};
		ctx.ui.notify(`Dossier: \`${dossierBin()} hook ${event}\` failed (${detail}); ${consequence[event]}.`, "warning");
	};

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

		// "reload" re-runs extensions against a session whose context is intact
		// and already holds the block injected when it started; re-injecting
		// would duplicate it.
		if (event.reason === "reload") return;

		const context = (await runHook("session-start", ctx))?.trim();
		if (!context) return;
		// A custom message participates in LLM context; "nextTurn" queues it for
		// the next prompt without interrupting or triggering a turn, which is the
		// same passive-injection shape the Claude Code SessionStart hook has.
		// display:false keeps the transcript clean — this is context for the
		// model, not output for the user.
		pi.sendMessage(
			{ customType: "dossier-context", content: context, display: false },
			{ deliverAs: "nextTurn" },
		);
	});

	pi.on("session_before_compact", async (_event, ctx) => {
		// Save before the context that would produce the state is discarded.
		// Returning nothing leaves compaction itself untouched: Dossier observes
		// this boundary, it does not get to cancel or rewrite it.
		await runHook("pre-compaction", ctx);
	});

	pi.on("session_shutdown", async (event, ctx) => {
		// "reload" swaps the extension runtime under a session that is still
		// going; it is not a session boundary and must not archive a transcript.
		if (event.reason !== "reload") {
			// Before clearPointer(): the CLI resolves this session's id and JSONL
			// through the pointer it is about to remove.
			await runHook("session-end", ctx);
		}
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
			lines.push(`dossier binary: ${dossierBin()}`);
			lines.push(
				`lifecycle hooks: ${hookErrors.size === 0 ? "session-start, session-end, pre-compaction" : "degraded"}`,
			);
			for (const [event, detail] of hookErrors) lines.push(`  ${event}: ${detail}`);
			if (lastError) lines.push(`last error: ${lastError}`);
			ctx.ui.notify(`Dossier\n${lines.join("\n")}`, lastError || hookErrors.size > 0 ? "warning" : "info");
		},
	});

	// Pi's native command is a short alias for the installed shared skill. The
	// skill contains the workflow; this bridge preserves the exact `/spark` UX
	// while allowing Pi to expand it as `/skill:spark`.
	pi.registerCommand("spark", {
		description: "Capture an unstructured thought as a new spark Dossier",
		handler: async (args) => {
			const prompt = args?.trim() ? `/skill:spark ${args}` : "/skill:spark";
			pi.sendUserMessage(prompt, { expandPromptTemplates: true });
		},
	});
}
