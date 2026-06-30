#!/usr/bin/env bash
set -euo pipefail

PROJECT_ID="${PROJECT_ID:-${1:-}}"
if [[ -z "${PROJECT_ID}" ]]; then
  echo "Usage: scripts/deploy-firebase.sh GOOGLE_CLOUD_PROJECT_ID" >&2
  exit 1
fi

if ! command -v firebase >/dev/null 2>&1; then
  echo "Firebase CLI bulunamadı. Kurulum: npm install -g firebase-tools" >&2
  exit 1
fi

firebase deploy --only hosting --project "${PROJECT_ID}"
echo "Dashboard: https://${PROJECT_ID}.web.app"
