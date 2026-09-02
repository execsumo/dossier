# Dossier Operating Instructions

- **Poll Monitors:** Evaluate `(Last polled: date)` in `## Active Monitors`. Fetch updates solely if outdated. Distill findings; update timestamp.
- **Eager Saves:** Execute `dossier_save` immediately upon material decisions or milestones. End-of-session batching: [Rejected].
- **Concurrency:** Inject `base_revision` into `dossier_save`. Mitigates concurrent TUI overwrite conflicts.
- **Artifacts:** Pass raw logs/transcripts as structured artifacts via `dossier_save`. Direct filesystem writes: [Rejected]. Prefer small, purpose-built artifacts (`decision_evidence`, `file_snapshot`, `query`, `link`) captured when the evidence appears, over one coarse end-of-session `transcript`.
- **Resolve Citations:** `dossier_artifact` fetches a cited artifact, or a cited span via `fragment: "L42-L68"`. Follow a `[src:]` pointer instead of guessing at what was compressed away, and instead of re-deriving what a past session already established.
- **Evidence Index:** `dossier_recall` returns `artifacts[]` (type, line count, cited/uncited) and `dossier_artifacts` lists it directly. An uncited-artifact warning means the distilled state has drifted off its own record: cite it or state why it is immaterial.
- **Cite Spans, Not Blobs:** Write `[src:art_<id>#L<a>-L<b>]` when the claim comes from part of a source. Ranges are validated against the artifact by `dossier doctor`.
- **Working Files:** Default loose deliverables, scratch files, and user-provided attachments to `<dossier_home>/<slug>/artifacts/` (direct filesystem write) to keep the dossier portable. Does not apply to source files inside an existing project/repo — leave those in place. Explicit user-specified path: overrides default.
- **Handoff:** Commit final state via `dossier_save`. Maintain actionable `## Next Steps`. Use `dossier_update` for isolated metadata mutations.
