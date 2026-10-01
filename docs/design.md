---
version: alpha
name: Dossier TUI
description: Design system extracted from the shipped Bubble Tea terminal interface; terminal capabilities and user theme take precedence where stated.
omitted:
  - section: typography
    reason: Terminal fonts, point sizes, and line heights are user-controlled; Dossier uses terminal cells and semantic emphasis instead.
  - section: rounded
    reason: TUI shapes are Unicode/terminal border glyphs, not pixel corner radii; the existing rounded border is a widget glyph choice.
  - section: spacing
    reason: Layout is measured in terminal cells and adapts to terminal dimensions rather than a fixed pixel scale.
colors:
  accent: "#A78BFA"
  accent-overlay: "#B18CFF"
  accent-link: "#B8A1FF"
  selection-text: "#FFFFFF"
  modal-background-ansi256: "53"
  status-spark: "#00D7D7"
  text-default: "terminal-default"
  text-light: "ansi-7"
  text-muted: "ansi-8"
  success: "ansi-2"
  warning: "ansi-3"
  error: "ansi-1"
  detail-empty: "#D6D3F0"
  overlay-hint: "#B9B6D6"
typography: {}
spacing: {}
components:
  title-banner:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.selection-text}"
    emphasis: bold
    horizontalPaddingCells: "2"
  section-heading:
    textColor: "{colors.accent}"
    emphasis: bold
  metadata-label:
    textColor: "{colors.accent}"
    emphasis: bold
  metadata-value:
    textColor: "{colors.text-default}"
  selected-row:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.selection-text}"
    emphasis: bold
    horizontalPaddingCells: "1"
  modal:
    backgroundColor: "{colors.modal-background-ansi256}"
    borderColor: "{colors.accent}"
    borderGlyph: rounded
    paddingCells: "1 2"
  link:
    textColor: "{colors.accent-link}"
    decoration: underline
  warning:
    textColor: "{colors.warning}"
    emphasis: bold
  error:
    textColor: "{colors.error}"
    emphasis: bold
  status-spark:
    textColor: "{colors.status-spark}"
    emphasis: bold
  status-define:
    textColor: "{colors.success}"
    emphasis: bold
  status-execute:
    textColor: "{colors.warning}"
  status-review:
    textColor: "{colors.accent}"
    emphasis: bold
  status-blocked:
    textColor: "{colors.error}"
    emphasis: bold
  status-done:
    textColor: "{colors.text-muted}"
---

# Dossier Design System

## Overview

Dossier is a local-first durable memory and delegation tool. Its TUI should feel like a focused, trustworthy workbench: information-dense but legible, calm rather than decorative, and explicit about state, risk, and available actions. Its visual language uses a restrained violet accent over the user's terminal theme, with familiar terminal colors for lifecycle and health semantics.

This document describes the existing product language and provides reusable direction for a sister app. It is an extraction, not a pixel-perfect mandate: preserve the principles and semantic roles, while adapting brand-specific naming and workflows where appropriate.

### TUI is the defining platform

Dossier is a Go single-binary application with a full-screen TUI built using Bubble Tea, Bubbles, Lip Gloss, and Glamour. It also has CLI and MCP adapters over the same core service; those surfaces share the product's terminology and semantic colors where styling exists, but this guide primarily governs the TUI. The terminal—not the application—chooses the font, font size, background, contrast mode, and available color depth. Layout therefore uses character cells, adapts to terminal width and height, and must remain understandable without color.

The TUI includes a dashboard table and a Kanban stage board over the same filtered data, Markdown detail and artifact views, inline editing, and contextual overlays for links, delegation terms, conflicts, and health. Filesystem notifications drive reactive refresh. Glamour renders Markdown using terminal-default document foreground/background so both light and dark terminal themes remain usable.

## Colors

Violet is the signature accent, not a fill color for the whole interface. The principal accent is `#A78BFA`; overlays use the closely related `#B18CFF` for headings and `#B8A1FF` for links. Existing implementations have these small variants; a sister app should choose one consistent accent unless a deliberate contrast or hierarchy distinction is needed.

Most body text inherits the terminal default. Muted and secondary text use ANSI 8 (bright black / terminal dark gray); general footer text uses ANSI 7 (light gray). This is intentional theme adaptation rather than a fixed light or dark palette. The modal background uses ANSI-256 color `53` so legacy and newer Lip Gloss renderers agree across ANSI-256 and truecolor terminals.

Semantic colors follow conventional terminal palette slots: green (ANSI 2) for positive/ready, yellow (ANSI 3) for warning and execute, red (ANSI 1) for blocked/error, and violet for review and selected emphasis. Spark has a dedicated cyan (`#00D7D7`). These colors reinforce labels and state; never make color the only carrier of meaning. Preserve the terminal-default foreground for long-form content.

The YAML values `terminal-default`, `ansi-*`, and `modal-background-ansi256` are semantic implementation references, not CSS colors. A web or native sister app should map them to its own accessible theme tokens rather than use these strings literally.

## Typography

Typography is inherited from the user's terminal. Do not prescribe or bundle a font, font size, line height, or letter spacing. Hierarchy is created through short labels, spacing, color, and restrained bold or italic emphasis:

- Use bold for screen titles, section headings, metadata labels, selected rows, and urgent states.
- Italics are occasional secondary emphasis (for example, the product subtitle or empty-state copy), not a second body style.
- Keep identifiers, dates, paths, and other operational values in readable plain text; do not rely on decorative typography or alignment that breaks with proportional terminal glyphs.
- Use the terminal's cell-width-aware layout and truncation behavior. Account for wide Unicode characters and ANSI escape sequences.

## Layout

The layout is responsive to the current terminal dimensions. The dashboard is the landing screen, with a compact table ordered by Dossier, Priority, Stage, Lead, and Due. The Kanban board is an alternate view, not a second data model. Detail metadata follows the same order and then adds Interfaces, Tokens, and Next. Keep the current scope/filter visible so narrowed results are not mistaken for the whole store.

Prefer clear grouping and concise labels over boxes around every element. Use one-cell-scale spacing for table rows and short controls; reserve larger gaps and padding for dialogs and distinct content sections. Footers sit at the terminal bottom and surface the most useful commands. Show only the key controls in the short footer; put the full key reference behind the help view. Overlays should have contextual controls and should not advertise actions from an inactive parent screen.

Always budget against terminal cell width and height. Wrap or window long lists, show indicators when content continues above or below, and keep every rendered line within the viewport. On narrow terminals, stack comparison columns rather than squeezing away meaning. Preserve enough room for actionable content before optional decoration.

## Elevation & Depth

The TUI has no shadows or simulated elevation. Hierarchy comes from terminal background layers, a restrained border, accent color, and selection inversion. Modal and overlay panels use the indexed violet-dark background (`53`) with a violet border; the main document stays on the terminal's own surface. Use overlays sparingly and keep the parent surface contextually apparent. Do not add gradients, drop shadows, or card-like elevation conventions that do not translate to terminal cells.

## Shapes

Shapes are made from terminal border glyphs, not CSS radii. The current editor and overlay panels use Lip Gloss rounded borders, with modest inner padding; tables use a simple bottom rule for their header. Treat rounded borders as a component treatment rather than a requirement for every region. Avoid decorative frames around ordinary text and avoid mixing multiple border styles in the same view without a functional reason.

## Components

### Dashboard and Kanban

- Keep the dashboard and Kanban as two views of one shared, filtered collection.
- The dashboard is a scan-first table: consistent columns, clear headers, and a distinct selected row (violet background, white bold foreground).
- Use an explicit lifecycle vocabulary and stable semantic styling: spark = cyan, define = green, execute = yellow, review = violet, blocked = red, done = muted. Text labels remain visible even when color is unavailable.
- Priority, due date, lead, and stage communicate actual work metadata. Avoid adding decorative scores or status indicators that are not part of the domain.
- Archived/done work may be visually secondary, but must remain findable and legible.

### Detail and Markdown

- Render the Distilled State as readable Markdown, with inherited terminal colors for body text and violet headings. Highlight the top-level heading as a violet banner with white text; use violet for subordinate headings and blockquote markers.
- Inline code uses cyan without a forced background, protecting contrast on both light and dark terminal themes.
- Keep the metadata block compact and consistently ordered. Distinguish labels from values with accent/bold versus terminal-default text.
- Citations, references, and archived evidence must remain navigable; preserve line-number and source cues as text.

### Editors, selectors, and overlays

- Use one combined editor for related metadata changes; clearly indicate focus and save/cancel behavior.
- A selected/focused option uses high-contrast white-on-violet emphasis. Other options remain plain; unavailable or secondary choices use muted text.
- Use rounded bordered panels with a modest violet border and indexed background for dialogs. Keep panel padding consistent and size the panel to the viewport.
- Contextual overlays preserve a path back to their parent and show only relevant shortcuts. Long lists are windowed with explicit more-above/more-below indicators.
- Conflicts must distinguish shared/current content from the preserved proposal, and present the available resolution actions plainly. Never style a conflict as if it were already resolved.

### Help, footer, and health

- Put a small set of high-value actions in the footer and a complete reference in the help view.
- Use violet bold for key names, light gray for descriptions, and muted gray for separators.
- Health is a concise, persistent summary; detailed diagnostics belong in a dedicated view. Warnings and errors are explicit, actionable, and never silently omitted.

## Do's and Don'ts

- Do inherit the terminal's default foreground and background for the main reading surface.
- Do use the violet accent consistently for headings, metadata labels, focus, and primary selection.
- Do pair semantic color with explicit text labels, icons, or wording so the UI survives monochrome and limited-color terminals.
- Do adapt layouts using terminal cell measurements, and test narrow, short, light-theme, dark-theme, ANSI-256, and truecolor terminals.
- Do keep CLI, MCP, and TUI behavior and terminology aligned; the TUI is an adapter, not a separate source of product rules.
- Do make warnings, errors, conflicts, and missing capabilities visible with useful next steps.
- Don't assume a fixed terminal palette, font, pixel size, or color depth.
- Don't use color as the sole distinction between lifecycle states or conflict sides.
- Don't fill large areas with the accent, add shadows, or apply decorative containers to every content block.
- Don't sacrifice readable content to fit a fixed desktop-style grid; reflow, wrap, or window it.
- Don't silently truncate important identifiers, dates, paths, error text, or operational content.
- Don't introduce new styles that make the sibling app look like a different product without an intentional brand decision.

## Reuse guidance

For a sister app, carry over the product-level conventions—terminal-native surfaces, violet as a restrained accent, conventional semantic colors, selected-row inversion, compact information hierarchy, contextual help, and truthful/actionable status. Reuse the component patterns only when the interaction is equivalent. Treat the Go/Charm stack, terminal color references, and cell-based spacing as implementation context; they define Dossier's options, but do not require an app on another platform to adopt Bubble Tea or Go.

This document follows the section structure and token-frontmatter approach of [Google Labs design.md spec](https://raw.githubusercontent.com/google-labs-code/design.md/refs/heads/main/docs/spec.md), adapted for a terminal UI. The palette and behaviors describe the current implementation; the prose separates observed conventions from recommendations for future reuse.
