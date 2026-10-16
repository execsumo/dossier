# Repair items 3 and 6 (plus the doctor half of item 4): evidence

Worktree: `/Users/hgill/projects/dossiers/.claude/worktrees/agent-a178ec404b69a7f1b`, base `7354587`. Nothing is committed. Every run used a fresh `mktemp` directory with both `HOME` and `DOSSIER_HOME` pointing into it, so no real store, harness settings, credentials or remote were touched.

Scope note: everything below was verified on synthetic fixtures only. The real store's artifact (4859 on-disk lines including frontmatter), its 17 files and its citations were **not** inspected. Whether that case is stale `lines:` metadata, a range past the body, or a body-versus-on-disk counting mix-up is inferred from these fixtures, not confirmed against the real files.

## Item 3: doctor and lead assignment disagree about the roster

### Root cause

The write path (`normalizeLeadUpdate`, `internal/core/service_team.go:38`) resolves a lead with `Roster.ResolvePerson`. That accepts a username, a display name, or a unique display-name prefix. It refuses ambiguous names, unknown people, former members and agents (B19), and stores the canonical username (B17).

Doctor (`internal/core/service.go:472-478` at base) used a different rule: `ResolvePerson` then `Has(storedValue)`. It also checked former members only by username key, through `displayLead`. As a result:

- a lead stored as a member's **display name** (the normal legacy value from the pre-roster `leads:` list in `config.yaml`) was reported as "not a member of the team roster", although it is one. `ls --lead psmith` silently misses that Dossier, because filters compare normalized usernames.
- an **ambiguous** display name was also reported as "not a member".
- an **agent** lead was not reported at all, although the write path refuses it.
- a **former** member stored by display name was reported as "not a member".

### Fix

There is now one classifier for both paths, `Roster.classifyLead` (`internal/core/roster.go:219`). `normalizeLeadUpdate` uses it (`service_team.go:54`) and keeps its exact error messages.

Doctor uses `Roster.storedLeadAdvisory` (`roster.go:238`, called at `service.go:473`). A stored lead is silent exactly when re-assigning it would succeed and store the same username; case and domain variants such as `ACME\PSmith` normalize to the same username and stay silent. Every other value gets one advisory naming the problem and the exact `dossier lead <slug> …` command.

Display-name handling:

- an **unambiguous** display name gets the resolved username in the advisory ("it resolves to Priya Shah (psmith)… Store the username: dossier lead pricing-review psmith");
- an **ambiguous** one lists the candidates and never picks one;
- an **agent** points to a Delegation Contract.

These are still advisories, not issues, so doctor is no harsher than before. Nothing is edited automatically.

### Tests (before fails, after passes)

`TestDoctorLeadAdvisoryMatchesAssignmentRules` (`internal/core/roster_doctor_test.go`) is a table of 8 stored values. For each one it also asserts that the write path's verdict agrees with the expected silent or advisory outcome, so doctor and assignment cannot drift apart again.

Before (the base code with the new tests copied in, at `/tmp/rep36/base2`):

```text
roster_doctor_test.go:120: advisory "...lead \"Priya Shah\" is not a member of the team roster" missing "lead \"Priya Shah\" is not stored as a roster username"
roster_doctor_test.go:120: advisory "...lead \"Sam Lee\" is not a member of the team roster" missing "...matches more than one team member: slee, sleeb"
roster_doctor_test.go:112: want exactly one lead advisory, got []          (agent lead)
roster_doctor_test.go:120: advisory "...lead \"Old User\" is not a member of the team roster" missing "...is a former team member (Old User)"
--- FAIL: TestDoctorLeadAdvisoryMatchesAssignmentRules
```

After: `--- PASS: TestDoctorLeadAdvisoryMatchesAssignmentRules`. The existing `TestDoctorAdvisesUnknownRosterLead` still passes.

### CLI reproduction

The script is `/tmp/rep36/repro3.sh <binary>`.

1. It creates a pre-roster store with `leads: [Priya Shah, Sam Lee, sitroom]` in config and promotes three Dossiers with those leads.
2. It runs `team add` for hgill, psmith "Priya Shah", slee "Sam Lee", sleeb "Sam Lee", and sitroom `--kind agent`. With no manager set, the bootstrap is open.
3. It shows the write path's verdict on each value.
4. It runs `ls --lead psmith`, then `doctor`.

The binaries were built with `go build -o /tmp/rep36/dossier-before ./cmd/dossier` at base, and `.../dossier-after` after the fix.

```text
bash /tmp/rep36/repro3.sh /tmp/rep36/dossier-before
  lead churn-study "Sam Lee"  -> [ambiguous_target] lead "Sam Lee" is ambiguous; candidates: slee, sleeb
  lead vm-ops sitroom         -> [invalid_frontmatter] lead "sitroom" is an agent; ...
  ls --lead psmith            -> No dossiers found.
  doctor:
- Warning: Dossier dos_… lead "Sam Lee" is not a member of the team roster
- Warning: Dossier dos_… lead "Priya Shah" is not a member of the team roster
  (nothing for the agent lead sitroom)

bash /tmp/rep36/repro3.sh /tmp/rep36/dossier-after
  doctor:
- Warning: Dossier dos_… lead "Sam Lee" matches more than one team member: slee, sleeb. Pick one: dossier lead churn-study <username>
- Warning: Dossier dos_… lead "Priya Shah" is not stored as a roster username; it resolves to Priya Shah (psmith), and lead filters such as `ls --lead psmith` miss it. Store the username: dossier lead pricing-review psmith
- Warning: Dossier dos_… lead "sitroom" is an agent (Sit Room); agents may own work only through a Delegation Contract. Assign a human lead: dossier lead vm-ops <username>

bash /tmp/rep36/apply3fix.sh /tmp/rep36/dossier-after /tmp/rep36/run.2HN45x/home   # runs the advised command
  Lead updated successfully.
  ls --lead psmith -> Pricing review  Priya Shah  ...  lead
  the pricing-review advisory is gone; the other two remain until a person decides
```

## Item 6: ## Files index and citation checks

### What was confirmed

- **Citations are measured against the artifact body, not the on-disk file.** Line 1 is the first line after the closing `---` of the frontmatter, which is the same numbering `dossier artifact -L` serves (`service_dossier.go`, `ReadArtifact` range path; `fsstore.go` `parseArtifactFile` / `countArtifactBodyLines`). A `wc -l` of the file therefore overstates the citable range by the frontmatter's length, roughly 13–15 lines for a typical artifact.
- **Root cause of the false clean result:** doctor bounded ranges by the recorded `lines:` frontmatter (`service.go:500-507` at base), while the fetch path bounds by the content. When `lines:` is stale, doctor passes a citation that `dossier artifact` refuses.
- The ## Files check worked, but its messages lacked a fix. A bare-path entry (no backticks) counts as unlisted, because `ParseFilesIndex` accepts only the `` - `files/<name>` `` form, and the advisory did not say so.

### Fix (no new port, no state edits)

- `service.go:507-527`: for each **cited** artifact, read it once through the existing `Store.ReadArtifact` and bound ranges by `artifactLineCount(content)`. If that differs from the recorded `lines:`, an advisory says "records lines: N but its content has M line(s)… The recorded line count is stale; citations are checked against the content." If the read fails, an advisory says so, and that artifact is checked against the recorded count. Uncited artifacts are not read.
- `provenance.go:107`: the out-of-range message now says the count is lines of content after the frontmatter, as `dossier artifact` numbers them, and to fix the range or the citation. The existing `has only N line(s)` substring is unchanged.
- `service.go:542`: a missing ## Files entry now says "Restore the file, or update or remove the entry with dossier_save."
- `workfiles.go:141`: the unlisted-files advisory now shows the exact entry form and that the path must be in backticks. The parser, the signature and the counts are unchanged.

Severity is unchanged: a broken citation range and a listed-but-missing file were already issues; the new metadata-mismatch line and the unlisted-file line are advisories.

One consequence to know: a fixture whose only problem was stale metadata used to report OK. It now fails, through the *existing* out-of-range issue, because the citation genuinely cannot be followed.

### Tests (before fails, after passes)

```text
before (/tmp/rep36/base2):
--- FAIL: TestDoctorBoundsCitationsByArtifactContent      provenance_test.go:223: a citation past the artifact's content must fail doctor
--- FAIL: TestDoctorChecksFilesIndex                      workfiles_test.go:119: missing-file issue must name the fix
--- FAIL: TestFSStoreDoctorBoundsCitationByContentNotStaleMetadata  fsstore_test.go:981: Doctor() trusted stale lines metadata: ok=true
after: all three PASS. Existing TestFSStoreLegacyLineCountMakesDoctorRejectOutOfRange and
TestFSStoreListArtifactsStreamCountsLegacyLinesWithoutMutation still pass.
```

The FS test also asserts three things:

- the on-disk file has more lines than the 3-line body;
- `#L3`, the last body line, is not flagged while `#L10-L12` is;
- doctor leaves the artifact file byte-identical.

### CLI reproduction

The script is `/tmp/rep36/repro6.sh <binary>`. It links a 5-line and a 3-line artifact, then sets the 3-line artifact's `lines:` to 40 to simulate stale metadata. It creates `files/deck.md`, `files/notes.md` and `files/plan.md`, and writes a Distilled State with these entries:

- citations `#L2-L4` (valid), `#L4-L9` (past the end) and `#L10-L12` on the stale artifact;
- ## Files: a backticked deck entry, a bare-path plan entry, a missing pptx, and a repo-identity entry.

```text
both binaries:  dossier artifact files-check <three> -L 10-12 -> [not_found] artifact … has 3 line(s); requested range starts at line 10

before doctor:
- … line 5 cites <five>#L4-L9 but the artifact has only 5 line(s)
- … lists files/missing.pptx under ## Files but it does not exist
- … has 2 file(s) in files/ not listed under ## Files …: files/notes.md, files/plan.md. List them so the next session can find the work.
  (no finding for the stale-metadata citation #L10-L12)

after doctor:
- … artifact <three> records lines: 40 but its content has 3 line(s) (numbered after the frontmatter, as `dossier artifact files-check <three>` prints them). The recorded line count is stale; citations are checked against the content.
- … line 5 cites <five>#L4-L9 but the artifact has only 5 line(s) of content (counted after its frontmatter, as `dossier artifact` numbers them); fix the range or the citation
- … line 6 cites <three>#L10-L12 but the artifact has only 3 line(s) of content (…); fix the range or the citation
- … lists files/missing.pptx under ## Files but it does not exist. Restore the file, or update or remove the entry with dossier_save.
- … has 2 file(s) … : files/notes.md, files/plan.md. List them …, one line each in the form - `files/<name>` (<kind>): <what it is>; the path must be in backticks to count as listed.
```

The repo-identity entry (`github.com/acme/api:docs/plan.md`) is skipped silently both before and after: it cannot be checked from the store. This is unchanged.

## Item 4 (doctor half): orphaned session bindings

The coordinator owns the port and store implementation. `SessionBindingStore` in `internal/core/ports.go:121-125` and `internal/store/session_bindings.go` with its test were copied byte-identical from `/tmp/dossier-repairs-858363ad`; main's copies win on integration.

Doctor (`service.go:579`) lists bindings and reports, as an **advisory**, each binding whose Dossier ID is in neither the listing nor a direct read. The message names the session ID and the missing Dossier ID, and gives the remedies: restore the Dossier, run `dossier switch <slug-or-id> --session <id>`, or, after inspection, remove only `sessions/<id>.json`. A listing error is also an advisory. Doctor never removes a binding.

Test: `TestDoctorReportsOrphanedSessionBindings` (`internal/core/doctor_bindings_test.go`). It failed before (`doctor_bindings_test.go:56: missing "Session \"sess_orphan\" is bound to dossier dos_gone, which does not exist"`) and passes after.

The CLI script is `/tmp/rep36/repro4orphan.sh <binary>`. It promotes two Dossiers, binds `sess-orphan` and `sess-kept`, then moves the orphan target's folder *out of the store* (kept, not deleted) as a stand-in for a manual removal.

```text
before doctor: (no session/binding lines)
after doctor:
- Warning: Session "sess-orphan" is bound to dossier dos_…, which does not exist. Restore that Dossier, rebind the session with `dossier switch <slug-or-id> --session sess-orphan`, or after inspection remove only the machine-local binding sessions/sess-orphan.json.
both: sessions/ still holds sess-kept.json and sess-orphan.json after doctor.
```

(The Pi advisory in both runs comes from a `pi` binary on PATH and is unrelated.)

## Independent combined-worktree CLI rerun

The coordinator re-ran the scripts against baseline `/tmp/dossier-before-item4`
and the combined build `/tmp/dossier-repairs-current`:

```sh
cd /tmp/dossier-repairs-858363ad
go build -o /tmp/dossier-repairs-current ./cmd/dossier
bash /tmp/rep36/repro3.sh /tmp/dossier-before-item4
bash /tmp/rep36/repro3.sh /tmp/dossier-repairs-current
bash /tmp/rep36/repro4orphan.sh /tmp/dossier-before-item4
bash /tmp/rep36/repro4orphan.sh /tmp/dossier-repairs-current
HOME=/tmp/rep36/run.Rl0xnV/home DOSSIER_HOME=/tmp/rep36/run.Rl0xnV/home/.dossier /tmp/dossier-repairs-current lead pricing-review psmith
HOME=/tmp/rep36/run.Rl0xnV/home DOSSIER_HOME=/tmp/rep36/run.Rl0xnV/home/.dossier /tmp/dossier-repairs-current ls --lead psmith
```

Observed the same before/after warnings above. Temp homes were `run.B3rZrJ`,
`run.Rl0xnV`, `run4.VBFUA5`, and `run4.ZYZG3o` under `/tmp/rep36/`.
The advised `lead` command succeeded and `ls --lead psmith` then listed Pricing
review as lead. Doctor left both bindings in place and only the repaired binary
reported the orphan. No real store was accessed.

## Verification

```text
gofmt -l .        -> (empty)
go vet ./...      -> clean
go test ./... -count=1 -> all packages ok (cli, core, store, sync, tui, mcp, …)
```

## Files changed

- Item 3: `internal/core/roster.go`, `internal/core/service_team.go`, `internal/core/service.go` (lead block), `internal/core/roster_doctor_test.go`.
- Item 6: `internal/core/service.go` (citation callback, missing-file text), `internal/core/provenance.go`, `internal/core/workfiles.go`, `internal/core/provenance_test.go`, `internal/core/workfiles_test.go`, `internal/store/fsstore_test.go`.
- Item 4 (doctor only): `internal/core/service.go` (binding block) and `internal/core/doctor_bindings_test.go`. The copied, coordinator-owned files are `internal/core/ports.go` (interface hunk only), `internal/store/session_bindings.go` and `internal/store/session_bindings_test.go`.
