param(
    [switch]$Execute,
    [string]$ConfirmProject = ""
)

$ErrorActionPreference = "Stop"
$project = "b2benerji-whatsapp-2026"
$accountId = "whatomate-firestore-preview"
$serviceAccount = "${accountId}@${project}.iam.gserviceaccount.com"
$secrets = @(
    "whatomate-encryption-key",
    "whatomate-jwt-secret",
    "whatomate-media-s3-key",
    "whatomate-media-s3-secret"
)

if ($Execute -and $ConfirmProject -ne $project) {
    throw "Safety guard: -ConfirmProject must exactly equal $project when -Execute is used."
}

$commands = @(
    @("iam", "service-accounts", "create", $accountId, "--project=$project", "--display-name=Whatomate Firestore preview runtime"),
    @("projects", "add-iam-policy-binding", $project, "--member=serviceAccount:$serviceAccount", "--role=roles/datastore.user", "--condition=None")
)
foreach ($secret in $secrets) {
    $commands += ,@("secrets", "add-iam-policy-binding", $secret, "--project=$project", "--member=serviceAccount:$serviceAccount", "--role=roles/secretmanager.secretAccessor", "--condition=None")
}

Write-Host "Target project  : $project"
Write-Host "Service account : $serviceAccount"
Write-Host "The production whatomate-runtime service account is not modified."

if (-not $Execute) {
    foreach ($arguments in $commands) {
        Write-Host ("gcloud " + ($arguments -join " "))
    }
    Write-Host "DRY RUN only. Re-run with -Execute -ConfirmProject $project after review."
    exit 0
}

$existingAccount = & gcloud iam service-accounts list --project=$project --filter="email=$serviceAccount" --format="value(email)"
if ($LASTEXITCODE -ne 0) { throw "Failed to inspect preview service account." }
if ($existingAccount -ne $serviceAccount) {
    $createArguments = $commands[0]
    & gcloud @createArguments
    if ($LASTEXITCODE -ne 0) { throw "Failed to create preview service account." }
}

for ($index = 1; $index -lt $commands.Count; $index++) {
    $iamArguments = $commands[$index]
    & gcloud @iamArguments
    if ($LASTEXITCODE -ne 0) { throw "IAM command $index failed with exit code $LASTEXITCODE." }
}
