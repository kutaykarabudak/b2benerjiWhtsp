param(
    [Parameter(Mandatory = $true)]
    [string]$Image,
    [switch]$Execute,
    [string]$ConfirmProject = ""
)

$ErrorActionPreference = "Stop"
$project = "b2benerji-whatsapp-2026"
$region = "europe-west1"
$service = "whatomate-firestore-preview"
$serviceAccount = "whatomate-firestore-preview@${project}.iam.gserviceaccount.com"

if ([string]::IsNullOrWhiteSpace($Image)) {
    throw "-Image is required."
}
if ($service -eq "whatomate") {
    throw "Safety guard: the production service name is forbidden."
}
if ($Execute -and $ConfirmProject -ne $project) {
    throw "Safety guard: -ConfirmProject must exactly equal $project when -Execute is used."
}

$arguments = @(
    "run", "deploy", $service,
    "--project=$project",
    "--region=$region",
    "--platform=managed",
    "--image=$Image",
	"--service-account=$serviceAccount",
    "--allow-unauthenticated",
    "--min-instances=0",
    "--max-instances=1",
    "--concurrency=20",
    "--cpu=1",
    "--memory=512Mi",
    "--timeout=60",
    "--cpu-throttling",
    "--clear-cloudsql-instances",
    "--clear-vpc-connector",
    "--set-env-vars=WHATOMATE_APP__NAME=B2B Enerji WhatsApp Preview,WHATOMATE_APP__ENVIRONMENT=production,WHATOMATE_APP__DEBUG=false,WHATOMATE_FIRESTORE__PROJECT_ID=$project,WHATOMATE_FIRESTORE__DATABASE_ID=(default),WHATOMATE_FIRESTORE__NAMESPACE=whatomate-v1-preview,WHATOMATE_COOKIE__SECURE=true,WHATOMATE_COOKIE__FIREBASE_HOSTING=true,WHATOMATE_JWT__ACCESS_EXPIRY_MINS=480,WHATOMATE_JWT__REFRESH_EXPIRY_DAYS=30,WHATOMATE_WHATSAPP__API_VERSION=v24.0,WHATOMATE_STORAGE__TYPE=s3,WHATOMATE_STORAGE__S3_BUCKET=b2benerji-whatsapp-2026-media,WHATOMATE_STORAGE__S3_REGION=europe-west1,WHATOMATE_STORAGE__S3_ENDPOINT=https://storage.googleapis.com",
    "--set-secrets=WHATOMATE_APP__ENCRYPTION_KEY=whatomate-encryption-key:latest,WHATOMATE_JWT__SECRET=whatomate-jwt-secret:latest,WHATOMATE_STORAGE__S3_KEY=whatomate-media-s3-key:latest,WHATOMATE_STORAGE__S3_SECRET=whatomate-media-s3-secret:latest"
)

Write-Host "Target project : $project"
Write-Host "Target service : $service"
Write-Host "Image          : $Image"
Write-Host "Production whatomate service and Firebase Hosting are not modified."

if (-not $Execute) {
    Write-Host "DRY RUN only. Re-run with -Execute -ConfirmProject $project after the image is reviewed."
    Write-Host ("gcloud " + ($arguments -join " "))
    exit 0
}

& gcloud @arguments
if ($LASTEXITCODE -ne 0) {
    throw "Preview deployment failed with exit code $LASTEXITCODE."
}

& gcloud run services describe $service --project=$project --region=$region --format="value(status.url)"
