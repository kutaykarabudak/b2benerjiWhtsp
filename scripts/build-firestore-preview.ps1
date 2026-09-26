param(
    [Parameter(Mandatory = $true)]
    [string]$Tag,
    [switch]$Execute,
    [string]$ConfirmProject = ""
)

$ErrorActionPreference = "Stop"
$project = "b2benerji-whatsapp-2026"
$region = "europe-west1"
$repository = "whatomate"
$image = "${region}-docker.pkg.dev/$project/$repository/whatomate-firestore:$Tag"

if ($Tag -notmatch '^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$') {
    throw "-Tag contains invalid container-tag characters."
}
if ($Execute -and $ConfirmProject -ne $project) {
    throw "Safety guard: -ConfirmProject must exactly equal $project when -Execute is used."
}

$arguments = @(
    "builds", "submit", ".",
    "--project=$project",
    "--region=$region",
    "--config=cloudbuild.firestore.yaml",
    "--substitutions=_IMAGE=$image"
)

Write-Host "Target project : $project"
Write-Host "Target image   : $image"
Write-Host "No Cloud Run service or Firebase Hosting route is changed by this build."

if (-not $Execute) {
    Write-Host ("gcloud " + ($arguments -join " "))
    Write-Host "DRY RUN only. Re-run with -Execute -ConfirmProject $project after review."
    exit 0
}

& gcloud @arguments
if ($LASTEXITCODE -ne 0) {
    throw "Cloud Build failed with exit code $LASTEXITCODE."
}

Write-Output $image
