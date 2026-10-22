# Deploy / update the stack on a VDS over SSH — one command, both cases.
#
# FIRST DEPLOY (server is empty apart from SSH):
#   cd deploy
#   .\push.ps1 -Server root@203.0.113.10 -Domain srunal-kz.duckdns.org `
#     -Bootstrap -BotToken 123456789:AAH... -BotUsername srunal_bot
#
# EVERY UPDATE AFTERWARDS (same command, without -Bootstrap):
#   .\push.ps1 -Server root@203.0.113.10 -Domain srunal-kz.duckdns.org
#
# What it does, in order:
#   1) packs the sources (never .env / node_modules / dist / var) into a tgz;
#   2) scp to the server;
#   3) on the server:
#        - installs Docker + ufw rules (only with -Bootstrap, skipped when present)
#        - creates backend/.env and deploy/.env (only with -Bootstrap, and only
#          when the file does not exist yet — existing configs are never touched)
#        - sets TELEGRAM_MINIAPP_SHORT_NAME when -MiniAppShortName is passed
#        - extracts the archive over $RemoteDir
#        - tags the current image as srunal-tgbot:prev (rollback point)
#        - docker compose --profile tls up -d --build
#        - waits for https://<Domain>/api/health
#
# Prerequisites: OpenSSH client on Windows (built in), DuckDNS name pointing at
# the server, bot token from @BotFather.

param(
  [Parameter(Mandatory = $true)][string]$Server,   # user@host (usually root@…)
  [string]$Domain = '',                            # required for -Bootstrap
  [string]$RemoteDir = '/opt/srunal',

  # first deploy
  [switch]$Bootstrap,
  [string]$BotToken = '',
  [string]$BotUsername = '',
  [string]$MiniAppShortName = '',
  [string]$PostgresPassword = '',
  [string]$AcmeEmail = '',

  # operational switches
  [switch]$SkipBuild,     # upload files only
  [switch]$PackOnly,      # stop before the upload (dry run of the packing)
  [switch]$DryRun,        # print the remote script and stop
  [switch]$ForceEnv,      # rewrite backend/.env and deploy/.env (backed up as .bak)
  [switch]$SyncAgentCredentials,  # copy ~/.cline providers.json into the container
  [switch]$ForceQr,       # overwrite the Kaspi QR in the container storage
  [string]$PaymentLink = '',      # set app_settings.payment_link (Kaspi QR link)
  [string]$AgentCredentials = "$env:USERPROFILE\.cline\data\settings\providers.json",
  [switch]$KeepArchive
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$archive = Join-Path $env:TEMP 'srunal-push.tgz'

if ($Bootstrap) {
  if (-not $Domain) { throw '-Bootstrap requires -Domain (e.g. -Domain srunal-kz.duckdns.org)' }
  if (-not $BotToken -or -not $BotUsername) { throw '-Bootstrap requires -BotToken and -BotUsername' }
  if (-not $PostgresPassword) {
    # Safe alphabet for shell quoting; change it later in deploy/.env if you like.
    $PostgresPassword = -join ((48..57) + (65..90) + (97..122) | Get-Random -Count 24 | ForEach-Object { [char]$_ })
  }
}
if ($Domain -and $Domain -match '://') { throw '-Domain must be a bare host name (no https://)' }
if ($PaymentLink -and $PaymentLink -notmatch '^https://[^\s''"<>]+$') {
  throw '-PaymentLink must be an https URL without spaces, quotes or angle brackets'
}

# --- 1. pack -----------------------------------------------------------------
Write-Host "== packing sources from $root" -ForegroundColor Cyan
if (Test-Path $archive) { Remove-Item $archive -Force }

$excludes = @(
  '--exclude=./miniapp/node_modules', '--exclude=./miniapp/dist',
  '--exclude=./backend/var', '--exclude=./backend/bin',
  '--exclude=./backend/.env', '--exclude=./deploy/.env',
  '--exclude=./.git', '--exclude=*.log', '--exclude=./srunal.tgz',
  '--exclude=./design/stitch/screens/*.png', '--exclude=./design/stitch/screens/*.html'
)
& tar -czf $archive @excludes -C $root .
if ($LASTEXITCODE -ne 0) { throw "tar failed with exit code $LASTEXITCODE" }
Write-Host ("== archive ready: {0} ({1} MB)" -f $archive, [math]::Round((Get-Item $archive).Length / 1MB, 1)) -ForegroundColor Green

if ($PackOnly) { Write-Host '== -PackOnly: stopping before upload' -ForegroundColor Yellow; return }

# --- 2. remote script --------------------------------------------------------
function Quote([string]$value) { "'" + ($value -replace "'", "'\''") + "'" }

$remote = New-Object System.Text.StringBuilder
function Line([string]$text) { [void]$remote.Append($text + "`n") }

Line 'set -e'
Line "FORCE_ENV=$(if ($ForceEnv) { 'yes' } else { 'no' })"

if ($Bootstrap) {
  Line @'
echo "== server setup"
export DEBIAN_FRONTEND=noninteractive
if ! command -v docker >/dev/null 2>&1; then
  apt-get update -qq
  apt-get install -y -qq ufw openssl curl git ca-certificates >/dev/null
  curl -fsSL https://get.docker.com | sh >/dev/null
fi
if command -v ufw >/dev/null 2>&1 && ufw status | grep -q inactive; then
  ufw allow 22/tcp >/dev/null; ufw allow 80/tcp >/dev/null; ufw allow 443/tcp >/dev/null
  ufw --force enable >/dev/null
  echo "   ufw enabled (22/80/443)"
fi
docker --version
'@
}

Line "mkdir -p $(Quote $RemoteDir)"
Line "tar -xzf /tmp/srunal.tgz -C $(Quote $RemoteDir) --overwrite"
Line 'rm -f /tmp/srunal.tgz'
# Extraction comes first: it creates backend/ and deploy/ for a fresh install,
# while .env files that already exist on the server are left untouched by tar.

if ($Bootstrap) {
  # The .env files are generated here and written verbatim on the server (the
  # heredocs use a quoted delimiter, so $ and # in a password survive). An
  # existing file is kept unless it looks incomplete — a half-written config
  # from an interrupted run would silently break the deployment.
  $pepper = -join (1..64 | ForEach-Object { '{0:x}' -f (Get-Random -Maximum 16) })
  Line @"
need_backend=no
if [ "`$FORCE_ENV" = yes ] || [ ! -s '$RemoteDir/backend/.env' ] \
   || ! grep -q '^TELEGRAM_BOT_TOKEN=..' '$RemoteDir/backend/.env' \
   || ! grep -q '^PUBLIC_BASE_URL=https://' '$RemoteDir/backend/.env' \
   || ! tr -d '\r' < '$RemoteDir/backend/.env' | cmp -s - '$RemoteDir/backend/.env'; then
  need_backend=yes
fi
if [ "`$need_backend" = yes ]; then
  [ -f '$RemoteDir/backend/.env' ] && cp '$RemoteDir/backend/.env' '$RemoteDir/backend/.env.bak'
  echo '== writing backend/.env'
  cat > '$RemoteDir/backend/.env' <<'EOF'
APP_ENV=prod
PUBLIC_BASE_URL=https://$Domain
TOKEN_PEPPER=$pepper
SEED_ON_START=false
TELEGRAM_BOT_ENABLED=true
TELEGRAM_BOT_TOKEN=$BotToken
TELEGRAM_BOT_USERNAME=$BotUsername
TELEGRAM_MINIAPP_SHORT_NAME=$MiniAppShortName
GENERATOR_MODE=agent
GENERATOR_PROVIDER=opencode-go
GENERATOR_MODEL=deepseek-v4.1-flash
GENERATOR_THINKING=high
EOF
else
  echo '== backend/.env looks complete, keeping it'
fi
need_deploy=no
if [ "`$FORCE_ENV" = yes ] || [ ! -s '$RemoteDir/deploy/.env' ] \
   || ! grep -q '^DOMAIN=.' '$RemoteDir/deploy/.env' \
   || ! grep -q '^POSTGRES_PASSWORD=..' '$RemoteDir/deploy/.env' \
   || ! tr -d '\r' < '$RemoteDir/deploy/.env' | cmp -s - '$RemoteDir/deploy/.env'; then
  need_deploy=yes
fi
if [ "`$need_deploy" = yes ]; then
  [ -f '$RemoteDir/deploy/.env' ] && cp '$RemoteDir/deploy/.env' '$RemoteDir/deploy/.env.bak'
  echo '== writing deploy/.env'
  cat > '$RemoteDir/deploy/.env' <<'EOF'
DOMAIN=$Domain
POSTGRES_PASSWORD=$PostgresPassword
ACME_EMAIL=$AcmeEmail
API_PORT=8080
GENERATOR_MODE=agent
EOF
else
  echo '== deploy/.env looks complete, keeping it'
fi
"@
}

if ($MiniAppShortName) {
  Line "sed -i 's|^TELEGRAM_MINIAPP_SHORT_NAME=.*|TELEGRAM_MINIAPP_SHORT_NAME=$MiniAppShortName|' '$RemoteDir/backend/.env' && echo '== Mini App short name set: $MiniAppShortName'"
}

Line "cd $(Quote "$RemoteDir/deploy")"

if (-not $SkipBuild) {
  Line @'
echo "== building and starting"
docker image inspect srunal-tgbot:latest >/dev/null 2>&1 && docker image tag srunal-tgbot:latest srunal-tgbot:prev || true
docker compose --profile tls up -d --build
docker compose ps
'@
} else {
  Line 'docker compose ps'
}

# The bundled Kaspi QR: installed once, when the storage has no QR yet (the
# panel upload stays the source of truth afterwards) or replaced when -ForceQr
# is passed. The working directory is $RemoteDir/deploy at this point, hence the
# relative "assets/..." path.
$replaceQr = if ($ForceQr) { '1' } else { '0' }
Line @"
REPLACE_QR=$replaceQr
if [ -f assets/kaspi_qr.png ]; then
  docker compose exec -T api mkdir -p /data/storage/settings >/dev/null 2>&1 || true
  if [ "`$REPLACE_QR" = "1" ]; then
    docker compose cp assets/kaspi_qr.png api:/data/storage/settings/kaspi_qr.png >/dev/null 2>&1 \
      && echo "== Kaspi QR replaced from the repository" || echo "== Kaspi QR copy failed (upload it in the panel)"
  elif docker compose exec -T api sh -c 'ls /data/storage/settings/kaspi_qr.* >/dev/null 2>&1'; then
    echo "== Kaspi QR already in storage, keeping it"
  else
    docker compose cp assets/kaspi_qr.png api:/data/storage/settings/kaspi_qr.png >/dev/null 2>&1 \
      && echo "== Kaspi QR installed from the repository" || echo "== Kaspi QR copy failed (upload it in the panel)"
  fi
fi
"@

# Optional: the Kaspi payment link lives in app_settings and is edited in the
# operator panel; -PaymentLink sets it during the deploy (idempotent).
if ($PaymentLink) {
  Line @"
docker compose exec -T db psql -U srunal -d srunal -c "update app_settings set payment_link='$PaymentLink' where id=1" \
  && echo "== payment link set (app_settings.payment_link)" || echo "== payment link update failed - set it in the operator panel"
"@
}

Line @'
if [ -f /tmp/cline-providers.json ]; then
  if docker compose cp /tmp/cline-providers.json api:/data/cline/settings/providers.json >/dev/null 2>&1; then
    echo "== agent credentials installed (data/cline/settings/providers.json)"
  else
    echo "== agent credentials copy failed, run: docker compose exec -it api cline auth"
  fi
  rm -f /tmp/cline-providers.json
fi
'@

if ($Domain) {
  Line @"
echo "== waiting for https://$Domain/api/health"
for i in `$(seq 1 36); do
  if curl -fsS --max-time 5 https://$Domain/api/health >/dev/null 2>&1; then
    echo "   OK: https://$Domain/api/health"
    exit 0
  fi
  sleep 5
done
echo "   health check did not pass in 3 minutes"
docker compose logs --tail 40 api
exit 1
"@
}

# Windows line endings break bash on the server (set -e\r is an invalid option,
# a heredoc terminator "EOF\r" never matches) — always send LF.
$scriptText = ($remote.ToString() -replace "`r`n", "`n" -replace "`r", "`n")
$scriptPath = Join-Path $env:TEMP 'srunal-deploy.sh'
[System.IO.File]::WriteAllText($scriptPath, $scriptText, [System.Text.UTF8Encoding]::new($false))
if ($scriptText -match '[^\x09\x0A\x20-\x7E]') {
  throw "the generated remote script contains non-ASCII characters; keep -Domain/-BotUsername plain"
}

if ($DryRun) {
  Write-Host '== remote script (-DryRun, not executed):' -ForegroundColor Yellow
  Write-Host "   saved to $scriptPath"
  Write-Host $scriptText
  return
}

# --- 3. upload and run -------------------------------------------------------
# Both files travel as files (scp), not as command-line arguments: Windows
# PowerShell 5.1 mangles quotes in long ssh arguments and truncates payloads.
Write-Host "== uploading to ${Server}:/tmp/" -ForegroundColor Cyan
& scp $archive "${Server}:/tmp/srunal.tgz"
if ($LASTEXITCODE -ne 0) { throw "scp failed with exit code $LASTEXITCODE" }
& scp $scriptPath "${Server}:/tmp/srunal-deploy.sh"
if ($LASTEXITCODE -ne 0) { throw "scp of the remote script failed with exit code $LASTEXITCODE" }

if ($SyncAgentCredentials) {
  if (-not (Test-Path $AgentCredentials)) {
    throw "-SyncAgentCredentials: $AgentCredentials not found (sign in locally with 'cline auth' first)"
  }
  Write-Host '== uploading the agent credentials (provider keys for the generation agent)' -ForegroundColor Cyan
  & scp $AgentCredentials "${Server}:/tmp/cline-providers.json"
  if ($LASTEXITCODE -ne 0) { throw "scp of the agent credentials failed with exit code $LASTEXITCODE" }
}

Write-Host '== running on the server' -ForegroundColor Cyan
& ssh $Server 'bash /tmp/srunal-deploy.sh; rc=$?; rm -f /tmp/srunal-deploy.sh; exit $rc'
if ($LASTEXITCODE -ne 0) { throw "remote step failed with exit code $LASTEXITCODE" }

if (-not $KeepArchive) { Remove-Item $archive -Force }
Write-Host '== done' -ForegroundColor Green
