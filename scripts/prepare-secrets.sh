#!/usr/bin/env bash
set -euo pipefail

PROJECT_ID="${PROJECT_ID:-${1:-}}"
if [[ -z "${PROJECT_ID}" ]]; then
  echo "Usage: PROJECT_ID=my-project scripts/prepare-secrets.sh" >&2
  exit 1
fi

gcloud config set project "${PROJECT_ID}" >/dev/null
gcloud services enable secretmanager.googleapis.com >/dev/null

put_secret() {
  local name="$1"
  local prompt="$2"
  local allow_generate="${3:-false}"
  local value=""

  read -r -s -p "${prompt}: " value
  echo
  if [[ -z "${value}" && "${allow_generate}" == "true" ]]; then
    value="$(openssl rand -base64 48 | tr -d '\n')"
    echo "${name} için güvenli değer üretildi."
  fi
  if [[ -z "${value}" ]]; then
    echo "${name} boş bırakılamaz." >&2
    return 1
  fi

  if gcloud secrets describe "${name}" --project "${PROJECT_ID}" >/dev/null 2>&1; then
    printf '%s' "${value}" | gcloud secrets versions add "${name}" --data-file=- --project "${PROJECT_ID}" >/dev/null
  else
    printf '%s' "${value}" | gcloud secrets create "${name}" --replication-policy=automatic --data-file=- --project "${PROJECT_ID}" >/dev/null
  fi
  unset value
  echo "${name} kaydedildi."
}

# Firestore runtime has no database, Redis, or bootstrap-admin password.
put_secret whatomate-encryption-key "Firestore'daki Meta tokenlarını şifreleyecek anahtar (Enter = üret)" true
put_secret whatomate-jwt-secret "Oturum JWT anahtarı (Enter = üret)" true

echo "Secret değerleri yalnızca Secret Manager'a yazıldı; repoya kaydedilmedi."
