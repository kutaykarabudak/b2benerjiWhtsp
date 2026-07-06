#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${1:-deploy/cloudrun.env}"
if [[ ! -f "${ENV_FILE}" ]]; then
  echo "Önce deploy/cloudrun.env.example dosyasını deploy/cloudrun.env olarak kopyalayıp doldurun." >&2
  exit 1
fi

set -a
source "${ENV_FILE}"
set +a

: "${PROJECT_ID:?PROJECT_ID gerekli}"
: "${CLOUD_SQL_CONNECTION:?CLOUD_SQL_CONNECTION gerekli}"
: "${DB_USER:?DB_USER gerekli}"
: "${DB_NAME:?DB_NAME gerekli}"
: "${ADMIN_EMAIL:?ADMIN_EMAIL gerekli}"
: "${REDIS_HOST:?REDIS_HOST gerekli}"

REGION="${REGION:-europe-west1}"
SERVICE="${SERVICE:-whatomate}"
FIREBASE_SITE="${FIREBASE_SITE:-${PROJECT_ID}}"
REDIS_PORT="${REDIS_PORT:-6379}"
REDIS_TLS="${REDIS_TLS:-false}"
REPOSITORY="whatomate"
IMAGE="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPOSITORY}/${SERVICE}:$(git rev-parse --short HEAD)"
CACHE_IMAGE="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPOSITORY}/${SERVICE}:latest"
RUNTIME_SA="whatomate-runtime@${PROJECT_ID}.iam.gserviceaccount.com"

gcloud config set project "${PROJECT_ID}" >/dev/null
gcloud services enable \
  artifactregistry.googleapis.com \
  cloudbuild.googleapis.com \
  run.googleapis.com \
  secretmanager.googleapis.com \
  sqladmin.googleapis.com >/dev/null

if ! gcloud artifacts repositories describe "${REPOSITORY}" --location "${REGION}" >/dev/null 2>&1; then
  gcloud artifacts repositories create "${REPOSITORY}" --repository-format=docker --location "${REGION}"
fi

if ! gcloud iam service-accounts describe "${RUNTIME_SA}" >/dev/null 2>&1; then
  gcloud iam service-accounts create whatomate-runtime --display-name="Whatomate Cloud Run runtime"
fi

for role in roles/cloudsql.client roles/secretmanager.secretAccessor; do
  gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
    --member="serviceAccount:${RUNTIME_SA}" \
    --role="${role}" \
    --condition=None >/dev/null
done

gcloud builds submit \
  --config cloudbuild.yaml \
  --substitutions="_IMAGE=${IMAGE},_CACHE_IMAGE=${CACHE_IMAGE}" \
  .

ENV_VARS="^~^WHATOMATE_APP__NAME=B2B Enerji WhatsApp~WHATOMATE_APP__ENVIRONMENT=production~WHATOMATE_APP__DEBUG=false~WHATOMATE_SERVER__ALLOWED_ORIGINS=https://${FIREBASE_SITE}.web.app,https://${FIREBASE_SITE}.firebaseapp.com~WHATOMATE_DATABASE__HOST=/cloudsql/${CLOUD_SQL_CONNECTION}~WHATOMATE_DATABASE__PORT=5432~WHATOMATE_DATABASE__USER=${DB_USER}~WHATOMATE_DATABASE__NAME=${DB_NAME}~WHATOMATE_DATABASE__SSL_MODE=disable~WHATOMATE_DEFAULT_ADMIN__EMAIL=${ADMIN_EMAIL}~WHATOMATE_REDIS__HOST=${REDIS_HOST}~WHATOMATE_REDIS__PORT=${REDIS_PORT}~WHATOMATE_REDIS__TLS=${REDIS_TLS}~WHATOMATE_COOKIE__SECURE=true~WHATOMATE_COOKIE__FIREBASE_HOSTING=true~WHATOMATE_RATE_LIMIT__ENABLED=true~WHATOMATE_RATE_LIMIT__TRUST_PROXY=true~WHATOMATE_WHATSAPP__API_VERSION=v24.0"

if [[ -n "${META_APP_ID:-}" ]]; then
  ENV_VARS+="~WHATOMATE_WHATSAPP__APP_ID=${META_APP_ID}"
fi
if [[ -n "${META_CONFIG_ID:-}" ]]; then
  ENV_VARS+="~WHATOMATE_WHATSAPP__CONFIG_ID=${META_CONFIG_ID}"
fi

# Firebase Hosting can't proxy WebSockets, so tell the frontend to connect WS
# directly to the Cloud Run service URL (stable across revisions).
WS_PUBLIC_URL="$(gcloud run services describe "${SERVICE}" --region "${REGION}" --format='value(status.url)' 2>/dev/null || true)"
if [[ -n "${WS_PUBLIC_URL}" ]]; then
  ENV_VARS+="~WHATOMATE_SERVER__WS_PUBLIC_URL=${WS_PUBLIC_URL}"
fi

SECRETS="WHATOMATE_APP__ENCRYPTION_KEY=whatomate-encryption-key:latest,WHATOMATE_JWT__SECRET=whatomate-jwt-secret:latest,WHATOMATE_DATABASE__PASSWORD=whatomate-db-password:latest,WHATOMATE_DEFAULT_ADMIN__PASSWORD=whatomate-admin-password:latest"
if gcloud secrets describe whatomate-redis-password >/dev/null 2>&1; then
  SECRETS+=",WHATOMATE_REDIS__PASSWORD=whatomate-redis-password:latest"
fi
if gcloud secrets describe whatomate-meta-app-secret >/dev/null 2>&1; then
  SECRETS+=",WHATOMATE_WHATSAPP__APP_SECRET=whatomate-meta-app-secret:latest"
fi

NETWORK_FLAGS=()
if [[ -n "${VPC_NETWORK:-}" && -n "${VPC_SUBNET:-}" ]]; then
  NETWORK_FLAGS+=(--network "${VPC_NETWORK}" --subnet "${VPC_SUBNET}" --vpc-egress private-ranges-only)
fi

gcloud run deploy "${SERVICE}" \
  --image "${IMAGE}" \
  --region "${REGION}" \
  --service-account "${RUNTIME_SA}" \
  --allow-unauthenticated \
  --ingress all \
  --port 8080 \
  --cpu 1 \
  --memory 1Gi \
  --min 1 \
  --max 1 \
  --concurrency 200 \
  --timeout 3600 \
  --no-cpu-throttling \
  --session-affinity \
  --add-cloudsql-instances "${CLOUD_SQL_CONNECTION}" \
  --set-env-vars "${ENV_VARS}" \
  --set-secrets "${SECRETS}" \
  --args=server,-config,config.toml,-migrate,-workers,1 \
  "${NETWORK_FLAGS[@]}"

echo "Cloud Run hazır. Sıradaki adım: scripts/deploy-firebase.sh ${PROJECT_ID}"
