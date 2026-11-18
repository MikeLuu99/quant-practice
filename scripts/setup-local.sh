#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
if ! git rev-parse --show-toplevel >/dev/null 2>&1; then
  git init -b main
fi
node scripts/install-codex-skills.mjs
printf '\nGit and Codex skills are ready. No remote, commit, or deployment was created.\n'
