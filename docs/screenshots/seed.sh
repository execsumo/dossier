#!/usr/bin/env bash
# Seed a throwaway Dossier store with demo data for README screenshots.
set -euo pipefail
H="${1:?usage: seed.sh <home-dir>}"
D="${DOSSIER_BIN:-dossier}"
d() { "$D" --home "$H" "$@" >/dev/null; }

mkdir -p "$H"   # NOTE: never run `init` here; it edits the real harness config even with --home
printf 'leads: [Alice, Bob, Carol]\ninterfaces: [Steerco, "1:1", Pricing WBR]\ntoken_limit: 100000\n' > "$H/config.yaml"

mk() { # slug lead priority due status next
  d promote "$1" --lead "$2" --priority "$3" --force \
    --distilled "## Objective
$7

## Done When
- Agreed plan signed off by $2
" --description "$7"
  d priority "$1" --priority "$3" --due "$4"
  d status "$1" "$5"
  d next "$1" "$6"
}
mk payments-migration  Alice high   2026-10-15 execute "Write the cutover runbook"        "Migrate payments to the new vendor API without downtime."
mk vendor-api-contract Bob   max    2026-10-08 blocked "Get legal review of revised SLA"   "Work out backward compatibility after the vendor contract change."
mk pricing-wbr-q4      Carol medium 2026-10-22 define  "Draft the pricing options table"   "Prepare Q4 pricing recommendation for the weekly business review."
mk onboarding-revamp   Alice medium 2026-11-05 review  "Collect feedback from three pilots" "Shorten new-hire onboarding from 6 weeks to 4."
mk data-retention-audit Bob  low    2026-11-30 spark   "Scope which systems are in play"   "Audit data retention against the new policy."
