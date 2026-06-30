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

put_optional_secret() {
  local name="$1"
  local prompt="$2"
  local value=""

  read -r -s -p "${prompt} (boş bırakırsanız atlanır): " value
  echo
  if [[ -z "${value}" ]]; then
    return 0
  fi

  if gcloud secrets describe "${name}" --project "${PROJECT_ID}" >/dev/null 2>&1; then
    printf '%s' "${value}" | gcloud secrets versions add "${name}" --data-file=- --project "${PROJECT_ID}" >/dev/null
  else
    printf '%s' "${value}" | gcloud secrets create "${name}" --replication-policy=automatic --data-file=- --project "${PROJECT_ID}" >/dev/null
  fi
  unset value
  echo "${name} kaydedildi."
}

put_secret whatomate-encryption-key "Veritabanındaki Meta tokenlarını şifreleyecek anahtar (Enter = üret)" true
put_secret whatomate-jwt-secret "Oturum JWT anahtarı (Enter = üret)" true
put_secret whatomate-db-password "Cloud SQL whatomate kullanıcısının parolası"
put_secret whatomate-admin-password "İlk dashboard admin parolası (en az 12 karakter)"
put_optional_secret whatomate-redis-password "Redis parolası"
put_optional_secret whatomate-meta-app-secret "Meta App Secret (Embedded Signup kullanacaksanız)"

echo "Secret değerleri yalnızca Secret Manager'a yazıldı; repoya kaydedilmedi."
