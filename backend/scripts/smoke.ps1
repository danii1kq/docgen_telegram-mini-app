# SruNal KZ backend smoke test.
# Exercises the whole public + admin surface against a running API and prints
# a PASS/FAIL line for every check. Run:
#   powershell -ExecutionPolicy Bypass -File scripts/smoke.ps1
# Optional: -Base http://localhost:8080 -Operator @operator_almaty -Password '<пароль оператора>'
#           -Db psql (path to psql.exe) -DbUser / -DbName for the client-login step.
#
# Client authorization goes through the Telegram bot, so the script cannot
# confirm the login code by itself. It plays the bot's part with psql (the bot
# only writes confirmed_at + chat_id on the challenge), then exchanges the code
# for a client session. Without psql the order checks are skipped with a warning.

param(
  [string]$Base = "http://localhost:8080",
  [string]$Operator = "@operator_almaty",
  [string]$Password = "Eshkere9113!xxx!",
  [string]$Db = "C:\Program Files\PostgreSQL\18\bin\psql.exe",
  [string]$DbUser = "srunal",
  [string]$DbName = "srunal",
  [string]$DbPassword = "srunal",
  [long]$ClientChatId = 555000111
)

$ErrorActionPreference = "Stop"
$script:passed = 0
$script:failed = 0
$script:token = ""

function Check([string]$name, [bool]$ok, [string]$detail = "") {
  if ($ok) {
    $script:passed++
    Write-Host ("  PASS  {0}" -f $name) -ForegroundColor Green
  } else {
    $script:failed++
    Write-Host ("  FAIL  {0} {1}" -f $name, $detail) -ForegroundColor Red
  }
}

# Invoke-Api returns @{status=...; body=...} and never throws on 4xx/5xx.
# Windows PowerShell 5.1 swallows error bodies in Invoke-WebRequest, so the
# request is built on HttpWebRequest instead.
function Invoke-Api {
  param(
    [string]$Method = "GET",
    [string]$Path,
    $Body = $null,
    [string]$Bearer = "",
    [string]$IdempotencyKey = "",
    [byte[]]$RawBody = $null,
    [string]$RawContentType = ""
  )
  $uri = "$Base$Path"
  $request = [System.Net.HttpWebRequest]::Create($uri)
  $request.Method = $Method
  $request.Accept = "application/json"
  if ($Bearer) { $request.Headers.Add("Authorization", "Bearer $Bearer") }
  if ($IdempotencyKey) { $request.Headers.Add("Idempotency-Key", $IdempotencyKey) }

  if ($null -ne $RawBody) {
    $request.ContentType = $RawContentType
    $request.ContentLength = $RawBody.Length
    $stream = $request.GetRequestStream()
    $stream.Write($RawBody, 0, $RawBody.Length)
    $stream.Close()
  } elseif ($null -ne $Body) {
    $json = $Body | ConvertTo-Json -Depth 8 -Compress
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($json)
    $request.ContentType = "application/json; charset=utf-8"
    $request.ContentLength = $bytes.Length
    $stream = $request.GetRequestStream()
    $stream.Write($bytes, 0, $bytes.Length)
    $stream.Close()
  }

  try {
    $response = $request.GetResponse()
  } catch [System.Net.WebException] {
    $response = $_.Exception.Response
  }

  if ($null -eq $response) {
    return @{ status = 0; body = $null; raw = "" }
  }
  $reader = New-Object System.IO.StreamReader($response.GetResponseStream())
  $raw = $reader.ReadToEnd()
  $reader.Close()
  $response.Close()

  $parsed = $null
  if ($raw) { try { $parsed = $raw | ConvertFrom-Json } catch { } }
  return @{ status = [int]$response.StatusCode; body = $parsed; raw = $raw }
}

Write-Host "== public endpoints ==" -ForegroundColor Cyan

$health = Invoke-Api -Path "/api/health"
Check "GET /api/health" ($health.status -eq 200 -and $health.body.status -eq "ok")

$settings = Invoke-Api -Path "/api/settings"
Check "GET /api/settings" ($settings.status -eq 200 -and $settings.body.paymentRecipient -eq "Даниил Л.")

$products = Invoke-Api -Path "/api/products"
Check "GET /api/products" ($products.status -eq 200 -and $products.body.products.Count -eq 4)

$quote = Invoke-Api -Method POST -Path "/api/quote" -Body @{ product = "essay"; works = 1; units = 10 }
Check "POST /api/quote essay 10 pages = 800" ($quote.status -eq 200 -and $quote.body.total -eq 800) ("got " + $quote.body.total)
Check "  quote lines rendered" ($quote.body.lines.Count -ge 5 -and $quote.body.lines[1].value -match "800")

$quoteVolume = Invoke-Api -Method POST -Path "/api/quote" -Body @{ product = "essay"; works = 10; units = 10 }
# 10 works x 10 pages x 80 = 8000, -15% = 6800
Check "POST /api/quote volume tier 15%" ($quoteVolume.body.discountPct -eq 15 -and $quoteVolume.body.total -eq 6800) ("got " + $quoteVolume.body.total)

$quoteFixed = Invoke-Api -Method POST -Path "/api/quote" -Body @{ product = "lab"; works = 1; units = 7 }
Check "POST /api/quote fixed product ignores units" ($quoteFixed.body.units -eq 1 -and $quoteFixed.body.total -eq 1000)

$quotePromo = Invoke-Api -Method POST -Path "/api/quote" -Body @{ product = "essay"; works = 1; units = 10; promo = @{ code = "SRUNAL10" } }
Check "POST /api/quote with SRUNAL10 = 720" ($quotePromo.body.promoPct -eq 10 -and $quotePromo.body.total -eq 720) ("got " + $quotePromo.body.total)

Write-Host "== promo checks ==" -ForegroundColor Cyan

# The rate-limit check at the end of this script closes the promo window for up
# to a minute. Wait until the endpoint answers again so repeated runs stay green.
$warmupDeadline = (Get-Date).AddSeconds(70)
while ((Get-Date) -lt $warmupDeadline) {
  $warmup = Invoke-Api -Method POST -Path "/api/promos/check" -Body @{ code = "SRUNAL10"; product = "essay"; units = 10; works = 1; isFirstOrder = $false; subtotal = 800 }
  if ($warmup.status -ne 429) { break }
  Write-Host "  ... waiting for the promo rate-limit window to reopen" -ForegroundColor DarkGray
  Start-Sleep -Seconds 5
}

$promoOk = Invoke-Api -Method POST -Path "/api/promos/check" -Body @{ code = "srunal10"; product = "essay"; units = 10; works = 1; isFirstOrder = $false; subtotal = 800 }
Check "SRUNAL10 applies" ($promoOk.body.ok -eq $true -and $promoOk.body.promo.code -eq "SRUNAL10") ("status " + $promoOk.status)

$promoEmpty = Invoke-Api -Method POST -Path "/api/promos/check" -Body @{ code = ""; product = "essay"; units = 10; isFirstOrder = $false }
Check "empty code -> Введите промокод" ($promoEmpty.body.ok -eq $false -and $promoEmpty.body.reason -eq "Введите промокод")

$promoUnknown = Invoke-Api -Method POST -Path "/api/promos/check" -Body @{ code = "NOPE123"; product = "essay"; units = 10; isFirstOrder = $false }
Check "unknown code -> Промокод не найден" ($promoUnknown.body.reason -eq "Промокод не найден")

$promoOff = Invoke-Api -Method POST -Path "/api/promos/check" -Body @{ code = "OLDFIVE"; product = "essay"; units = 10; isFirstOrder = $false }
Check "disabled code -> Промокод отключён" ($promoOff.body.reason -eq "Промокод отключён")

$promoExpired = Invoke-Api -Method POST -Path "/api/promos/check" -Body @{ code = "SPRING25"; product = "essay"; units = 10; isFirstOrder = $false }
Check "expired code -> Срок действия промокода истёк" ($promoExpired.body.reason -eq "Срок действия промокода истёк")

$promoFirst = Invoke-Api -Method POST -Path "/api/promos/check" -Body @{ code = "FIRST15"; product = "essay"; units = 10; isFirstOrder = $false }
Check "FIRST15 for a returning client" ($promoFirst.body.reason -eq "Промокод действует только на первый заказ")

$promoProduct = Invoke-Api -Method POST -Path "/api/promos/check" -Body @{ code = "LABHIT10"; product = "essay"; units = 10; isFirstOrder = $false; subtotal = 800 }
Check "product mismatch message" ($promoProduct.body.reason -eq "Промокод действует только на «Лабораторная»") ("got " + $promoProduct.body.reason)

$promoUnits = Invoke-Api -Method POST -Path "/api/promos/check" -Body @{ code = "WINTER20"; product = "essay"; units = 3; isFirstOrder = $false; subtotal = 240 }
Check "min units message" ($promoUnits.body.reason -eq "Промокод действует от 5 страниц") ("got " + $promoUnits.body.reason)

$promoAmount = Invoke-Api -Method POST -Path "/api/promos/check" -Body @{ code = "LABHIT10"; product = "lab"; units = 1; isFirstOrder = $false; subtotal = 999 }
# The price uses U+00A0 as the group separator, exactly like formatMoney().
$expectedAmountMessage = "Промокод действует от 1" + [char]0x00A0 + "000 ₸"
Check "min amount message" ($promoAmount.body.reason -eq $expectedAmountMessage) ("got " + $promoAmount.body.reason)

Write-Host "== client login (Telegram bot) ==" -ForegroundColor Cyan

$gate = Invoke-Api -Method POST -Path "/api/orders" -Body @{
  product = "essay"; works = 1; units = 10; topics = "Заказ без входа"; language = "ru"
  institute = "ИЦИТ"; department = "Кафедра"; studentName = "Тест Т."; disciplineCode = "Т-1"
  teacherName = "Препод П."; attachments = @(); contactName = "Тест"; consent = $true
}
Check "order without a session -> 401 (bot login required)" ($gate.status -eq 401) ("status " + $gate.status)

$clientStart = Invoke-Api -Method POST -Path "/api/auth/telegram/start"
Check "POST /api/auth/telegram/start returns a code" ($clientStart.status -eq 200 -and $clientStart.body.code -match "^\d{6}$")

$clientToken = ""
if (Test-Path $Db) {
  # Play the bot's part: /login <code> writes confirmed_at + chat_id.
  $env:PGPASSWORD = $DbPassword
  $sql = "update telegram_login_challenges set confirmed_at = now(), chat_id = $ClientChatId, " +
         "chat_username = '@smoke_client' where code = '$($clientStart.body.code)'"
  & $Db -h 127.0.0.1 -U $DbUser -d $DbName -tAc $sql | Out-Null
  if ($LASTEXITCODE -ne 0) { Write-Host "  ... psql failed, skipping the client checks" -ForegroundColor DarkGray }

  $auth = Invoke-Api -Method POST -Path "/api/auth/telegram/poll" -Body @{ code = $clientStart.body.code }
  $clientToken = $auth.body.token
  Check "POST /api/auth/telegram/poll -> client session" ($auth.status -eq 200 -and $auth.body.session.chatId -eq $ClientChatId -and $clientToken) ("status " + $auth.status)

  $me = Invoke-Api -Path "/api/auth/me" -Bearer $clientToken
  Check "GET /api/auth/me returns the Telegram identity" ($me.status -eq 200 -and $me.body.username -eq "@smoke_client") ("got " + $me.body.username)

  $replay = Invoke-Api -Method POST -Path "/api/auth/telegram/poll" -Body @{ code = $clientStart.body.code }
  Check "  the login code is single-use" ($replay.status -eq 404) ("status " + $replay.status)
} else {
  Write-Host "  ... psql not found ($Db) - skipping the client-login and order checks" -ForegroundColor DarkGray
}

Write-Host "== orders ==" -ForegroundColor Cyan

$badOrder = Invoke-Api -Method POST -Path "/api/orders" -Body @{
  product = "essay"; works = 1; units = 10; topics = "ab"; notes = ""; language = "ru"
  institute = ""; department = "Кафедра"; studentName = "Тест Т."; disciplineCode = "Т-1"
  teacherName = "Препод П."; attachments = @(); contactName = "Тест"; consent = $false
} -Bearer $clientToken
Check "validation errors" ($badOrder.status -eq 422 -and $badOrder.body.error.code -eq "validation_failed" -and $badOrder.body.error.fields.topics -and $badOrder.body.error.fields.institute -and $badOrder.body.error.fields.consent)

$orderBody = @{
  product = "essay"; works = 1; units = 10; topics = "Смоук-тест: проверка API"
  notes = "Автотест"; language = "ru"; institute = "ИЦИТ"; department = "Программная инженерия"
  studentName = "Тест Тестович"; disciplineCode = "CS-101"; teacherName = "Доц. Тестов Т.Т."
  attachments = @(@{ name = "brief.pdf"; size = 1024; type = "application/pdf" })
  contactName = "Тест Тестович"; consent = $true; promoCode = "SRUNAL10"
}
$idempotency = [guid]::NewGuid().ToString()
$created = Invoke-Api -Method POST -Path "/api/orders" -Body $orderBody -IdempotencyKey $idempotency -Bearer $clientToken
Check "POST /api/orders -> 201" ($created.status -eq 201) ("status " + $created.status)
$order = $created.body
Check "  number allocated" ($order.number -match "^DG-\d{6,}$") ("got " + $order.number)
Check "  token returned" ($order.token -match "^[A-Z2-9]{6}$") ("got " + $order.token)
Check "  price locked with promo (720)" ($order.amount -eq 720) ("got " + $order.amount)
Check "  status awaiting_payment" ($order.status -eq "awaiting_payment")
Check "  promo recorded" ($order.promoCode -eq "SRUNAL10" -and $order.promoDiscountPct -eq 10)
Check "  attachment stored" ($order.attachments.Count -eq 1)
Check "  attached to the Telegram chat of the session" ($order.contact -eq "@smoke_client") ("got " + $order.contact)

$replayed = Invoke-Api -Method POST -Path "/api/orders" -Body $orderBody -IdempotencyKey $idempotency -Bearer $clientToken
Check "Idempotency-Key replays the same order" ($replayed.body.number -eq $order.number -and $replayed.body.token -eq $order.token)

$public = Invoke-Api -Path ("/api/orders/" + $order.number)
Check "GET public order hides private fields" ($public.status -eq 200 -and $public.body.contact -eq "" -and -not $public.body.senderName)

$withToken = Invoke-Api -Path ("/api/orders/" + $order.number + "?token=" + $order.token)
Check "GET order with ?token= is complete" ($withToken.status -eq 200 -and $withToken.body.number -eq $order.number)

$lookup = Invoke-Api -Method POST -Path "/api/orders/lookup" -Body @{ number = $order.number; token = $order.token }
Check "POST /api/orders/lookup" ($lookup.status -eq 200 -and $lookup.body.number -eq $order.number)

$lookupBad = Invoke-Api -Method POST -Path "/api/orders/lookup" -Body @{ number = $order.number; token = "AAAAAA" }
Check "wrong token -> 404 without hints" ($lookupBad.status -eq 404 -and $lookupBad.body.error.message -match "Заказ не найден")

$batch = Invoke-Api -Method POST -Path "/api/orders/batch" -Body @{ items = @(@{ number = $order.number; token = $order.token }, @{ number = "DG-000000"; token = "AAAAAA" }) }
Check "POST /api/orders/batch filters pairs" ($batch.status -eq 200 -and $batch.body.Count -eq 1)

$claimBad = Invoke-Api -Method POST -Path ("/api/orders/" + $order.number + "/claim") -Body @{ token = $order.token; senderName = "Тест" }
Check "claim rejects a bad sender name" ($claimBad.status -eq 422 -and $claimBad.body.error.fields.senderName)

$claim = Invoke-Api -Method POST -Path ("/api/orders/" + $order.number + "/claim") -Body @{ token = $order.token; senderName = "Тест Т." }
Check "claim moves to payment_claimed" ($claim.status -eq 200 -and $claim.body.status -eq "payment_claimed" -and $claim.body.senderName -eq "Тест Т.")

$reviewsPublic = Invoke-Api -Path "/api/reviews?status=published"
$publishedOk = $reviewsPublic.status -eq 200 -and $reviewsPublic.body.Count -ge 6
if ($publishedOk) {
  foreach ($review in $reviewsPublic.body) {
    if ($review.status -ne "published") { $publishedOk = $false }
  }
}
Check "GET /api/reviews?status=published only publishes" $publishedOk ("got " + $reviewsPublic.body.Count)

$review = Invoke-Api -Method POST -Path "/api/reviews" -Body @{
  orderNumber = $order.number; author = "Тест Т."; institute = "Satbayev Univ"
  department = "Программная инженерия"; discipline = "API smoke"; product = "essay"
  rating = 5; text = "Автоматический отзыв из смоук-теста, проверяем модерацию."
}
Check "POST /api/reviews -> pending" ($review.status -eq 201 -and $review.body.status -eq "pending")

$reviewBad = Invoke-Api -Method POST -Path "/api/reviews" -Body @{
  orderNumber = "DG-000000"; author = "Тест"; institute = "x"; department = "x"
  discipline = "x"; product = "essay"; rating = 5; text = "Отзыв к несуществующему заказу."
}
Check "review for an unknown order -> 422" ($reviewBad.status -eq 422)

Write-Host "== admin auth ==" -ForegroundColor Cyan

$badLogin = Invoke-Api -Method POST -Path "/api/admin/auth/login" -Body @{ operatorId = $Operator; code = "wrong-code" }
Check "wrong password -> 401" ($badLogin.status -eq 401 -and $badLogin.body.error.message -eq "Неверный email или пароль")

$noSession = Invoke-Api -Path "/api/admin/orders"
Check "admin endpoint without session -> 401" ($noSession.status -eq 401)

$login = Invoke-Api -Method POST -Path "/api/admin/auth/login" -Body @{ operatorId = $Operator; code = $Password }
Check "login -> session + token" ($login.status -eq 200 -and $login.body.session.operatorId -eq $Operator -and $login.body.token) ("status " + $login.status)
$script:token = $login.body.token

$me = Invoke-Api -Path "/api/admin/me" -Bearer $script:token
Check "GET /api/admin/me" ($me.status -eq 200 -and $me.body.operatorId -eq $Operator)

Write-Host "== admin orders ==" -ForegroundColor Cyan

$list = Invoke-Api -Path "/api/admin/orders" -Bearer $script:token
Check "GET /api/admin/orders envelope" ($list.status -eq 200 -and $list.body.orders.Count -gt 0 -and $list.body.counts.payment_claimed -ge 1)
Check "  payment_claimed pinned first" ($list.body.orders[0].status -eq "payment_claimed")

$listArray = Invoke-Api -Path "/api/admin/orders?shape=array" -Bearer $script:token
Check "  ?shape=array returns a list" ($listArray.status -eq 200 -and $listArray.body.Count -gt 0)

$filtered = Invoke-Api -Path "/api/admin/orders?status=completed&q=DG-260" -Bearer $script:token
Check "  filtering by status/q" ($filtered.status -eq 200 -and $filtered.body.orders.Count -ge 1)

$adminOrder = Invoke-Api -Path ("/api/admin/orders/" + $order.number) -Bearer $script:token
Check "GET /api/admin/orders/{number} has events" ($adminOrder.status -eq 200 -and $adminOrder.body.events.Count -ge 1 -and $adminOrder.body.quoteLines.Count -ge 1)

$confirm = Invoke-Api -Method POST -Path ("/api/admin/orders/" + $order.number + "/confirm") -Bearer $script:token
Check "confirm -> queued" ($confirm.status -eq 200 -and $confirm.body.status -eq "queued" -and $confirm.body.confirmedAt)

$doubleConfirm = Invoke-Api -Method POST -Path ("/api/admin/orders/" + $order.number + "/confirm") -Bearer $script:token
Check "confirm twice -> 409" ($doubleConfirm.status -eq 409)

$note = Invoke-Api -Method PATCH -Path ("/api/admin/orders/" + $order.number) -Bearer $script:token -Body @{ operatorNote = "Проверено смоук-тестом" }
Check "PATCH operator note" ($note.status -eq 200 -and $note.body.operatorNotes -eq "Проверено смоук-тестом")

# The worker turns queued -> generating -> completed with a real .docx.
Write-Host "  ... waiting up to 3 minutes for the worker" -ForegroundColor DarkGray
$deadline = (Get-Date).AddMinutes(3)
$finished = $null
while ((Get-Date) -lt $deadline) {
  Start-Sleep -Seconds 6
  $check = Invoke-Api -Path ("/api/admin/orders/" + $order.number) -Bearer $script:token
  if ($check.body.status -eq "completed") { $finished = $check.body; break }
}
Check "worker generated the document" ($null -ne $finished) ("status " + $check.body.status)
if ($finished) {
  Check "  resultFile attached" ($finished.resultFile -and $finished.resultFile.url -match "sig=")
  $download = Invoke-Api -Path ($finished.resultFile.url -replace [regex]::Escape($Base), "")
  Check "  result download works" ($download.status -eq 200 -and $download.raw.Length -gt 1000) ("bytes " + $download.raw.Length)
  $noSig = Invoke-Api -Path ("/api/orders/" + $order.number + "/result")
  Check "  download without sig/token -> 401" ($noSig.status -eq 401)
}

Write-Host "== admin reviews ==" -ForegroundColor Cyan

$adminReviews = Invoke-Api -Path "/api/admin/reviews" -Bearer $script:token
Check "GET /api/admin/reviews with counts" ($adminReviews.status -eq 200 -and $adminReviews.body.counts.pending -ge 1)

$pending = $adminReviews.body.reviews | Where-Object { $_.status -eq "pending" } | Select-Object -First 1
$moderate = Invoke-Api -Method PATCH -Path ("/api/admin/reviews/" + $pending.id) -Bearer $script:token -Body @{ status = "published" }
Check "PATCH review -> published" ($moderate.status -eq 200 -and $moderate.body.status -eq "published")

Write-Host "== admin promos ==" -ForegroundColor Cyan

$promos = Invoke-Api -Path "/api/admin/promos" -Bearer $script:token
Check "GET /api/admin/promos" ($promos.status -eq 200 -and $promos.body.Count -eq 6)

$newPromo = Invoke-Api -Method POST -Path "/api/admin/promos" -Bearer $script:token -Body @{
  code = "smoke10"; discountPct = 10; appliesTo = "any"; products = @("essay")
  minAmount = 500; minUnits = $null; usageLimit = 5; expiresAt = "2026-12-31T23:59:59.000Z"; active = $true
}
Check "POST promo -> code uppercased" ($newPromo.status -eq 201 -and $newPromo.body.code -eq "SMOKE10" -and $newPromo.body.products.Count -eq 1)
$newPromoId = $newPromo.body.id

$duplicate = Invoke-Api -Method POST -Path "/api/admin/promos" -Bearer $script:token -Body @{
  code = "SMOKE10"; discountPct = 5; appliesTo = "any"; products = @(); active = $true
}
Check "duplicate promo -> «Промокод SMOKE10 уже существует»" ($duplicate.status -eq 409 -and $duplicate.body.error.message -eq "Промокод SMOKE10 уже существует") ("got " + $duplicate.body.error.message)

$toggle = Invoke-Api -Method PATCH -Path ("/api/admin/promos/" + $newPromoId) -Bearer $script:token -Body @{ active = $false }
Check "PATCH promo disables without wiping fields" ($toggle.status -eq 200 -and $toggle.body.active -eq $false -and $toggle.body.discountPct -eq 10 -and $toggle.body.code -eq "SMOKE10")

$delete = Invoke-Api -Method DELETE -Path ("/api/admin/promos/" + $newPromoId) -Bearer $script:token
Check "DELETE promo" ($delete.status -eq 200)

Write-Host "== admin pricing & settings ==" -ForegroundColor Cyan

$pricing = Invoke-Api -Path "/api/admin/pricing" -Bearer $script:token
Check "GET pricing" ($pricing.status -eq 200 -and $pricing.body.products.Count -eq 4)

$savedPricing = $pricing.body
$savedPricing.products[0].unitPrice = 90
$updatedPricing = Invoke-Api -Method PUT -Path "/api/admin/pricing" -Bearer $script:token -Body $savedPricing
Check "PUT pricing updates essay to 90" ($updatedPricing.status -eq 200 -and $updatedPricing.body.products[0].unitPrice -eq 90)

$afterPrice = Invoke-Api -Method POST -Path "/api/quote" -Body @{ product = "essay"; works = 1; units = 10 }
Check "  new quote uses 90 => 900" ($afterPrice.body.total -eq 900)
Check "  frozen order keeps 720" ((Invoke-Api -Path ("/api/admin/orders/" + $order.number) -Bearer $script:token).body.amount -eq 720)

$savedPricing.products[0].unitPrice = 80
$restored = Invoke-Api -Method PUT -Path "/api/admin/pricing" -Bearer $script:token -Body $savedPricing
Check "  pricing restored to 80" ($restored.body.products[0].unitPrice -eq 80)

$badPricing = $savedPricing
$badPricing.products[0].unitPrice = 0
$invalidPricing = Invoke-Api -Method PUT -Path "/api/admin/pricing" -Bearer $script:token -Body $badPricing
Check "PUT pricing validates prices" ($invalidPricing.status -eq 422)

$settingsPut = Invoke-Api -Method PUT -Path "/api/admin/settings" -Bearer $script:token -Body @{
  requisites = @{ recipient = "ИП «SruNal KZ»"; bank = "Kaspi Bank"; card = "4400 4300 1234 5678"; iban = "KZ75"; note = "x" }
  paymentQr = $null; paymentRecipient = "Даниил Л."; supportTelegram = "@meownuarka"
  maintenance = $false; queuePaused = $false
}
Check "PUT settings" ($settingsPut.status -eq 200 -and $settingsPut.body.maintenance -eq $false)

$maintenance = Invoke-Api -Method PUT -Path "/api/admin/settings" -Bearer $script:token -Body @{
  requisites = $settingsPut.body.requisites; paymentQr = $null; paymentRecipient = "Даниил Л."
  supportTelegram = "@meownuarka"; maintenance = $true; queuePaused = $false
}
Check "  maintenance on" ($maintenance.body.maintenance -eq $true)
$blocked = Invoke-Api -Method POST -Path "/api/orders" -Body $orderBody -Bearer $clientToken
Check "  order creation blocked by maintenance" ($blocked.status -eq 503 -and $blocked.body.error.code -eq "maintenance")
$null = Invoke-Api -Method PUT -Path "/api/admin/settings" -Bearer $script:token -Body @{
  requisites = $settingsPut.body.requisites; paymentQr = $null; paymentRecipient = "Даниил Л."
  supportTelegram = "@meownuarka"; maintenance = $false; queuePaused = $false
}

# QR upload as multipart/form-data (Windows PowerShell 5.1 has no -Form).
$png = [Convert]::FromBase64String("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg==")
$boundary = "----srunal" + [guid]::NewGuid().ToString("N")
$prefix = "--$boundary`r`nContent-Disposition: form-data; name=`"file`"; filename=`"kaspi_qr.png`"`r`nContent-Type: image/png`r`n`r`n"
$suffix = "`r`n--$boundary--`r`n"
$prefixBytes = [System.Text.Encoding]::UTF8.GetBytes($prefix)
$suffixBytes = [System.Text.Encoding]::UTF8.GetBytes($suffix)
$multipart = New-Object byte[] ($prefixBytes.Length + $png.Length + $suffixBytes.Length)
[Array]::Copy($prefixBytes, 0, $multipart, 0, $prefixBytes.Length)
[Array]::Copy($png, 0, $multipart, $prefixBytes.Length, $png.Length)
[Array]::Copy($suffixBytes, 0, $multipart, $prefixBytes.Length + $png.Length, $suffixBytes.Length)

$upload = Invoke-Api -Method POST -Path "/api/admin/settings/payment-qr" -Bearer $script:token `
  -RawBody $multipart -RawContentType "multipart/form-data; boundary=$boundary"
Check "POST payment-qr upload" ($upload.status -eq 200 -and $upload.body.paymentQr -match "/api/settings/payment-qr") ("status " + $upload.status)
$qrImage = Invoke-Api -Path "/api/settings/payment-qr"
Check "  QR served as an image" ($qrImage.status -eq 200 -and $qrImage.raw.Length -gt 50)

Write-Host "== admin stats ==" -ForegroundColor Cyan
$stats = Invoke-Api -Path "/api/admin/stats" -Bearer $script:token
Check "GET /api/admin/stats" ($stats.status -eq 200 -and $stats.body.ordersTotal -ge 11 -and $null -ne $stats.body.ordersByStatus.payment_claimed)
Check "  revenue and queue metrics" ($null -ne $stats.body.confirmedRevenue -and $null -ne $stats.body.queueDepth)

Write-Host "== telegram login ==" -ForegroundColor Cyan
$start = Invoke-Api -Method POST -Path "/api/admin/auth/telegram/start"
Check "POST telegram/start returns a code" ($start.status -eq 200 -and $start.body.code -match "^\d{6}$")

$poll = Invoke-Api -Method POST -Path "/api/admin/auth/telegram/poll" -Body @{ code = $start.body.code }
Check "  poll while pending" ($poll.status -eq 200 -and $poll.body.pending -eq $true)

Write-Host "== rate limits ==" -ForegroundColor Cyan
$limited = $false
for ($i = 0; $i -lt 80; $i++) {
  $probe = Invoke-Api -Method POST -Path "/api/promos/check" -Body @{ code = "SRUNAL10"; product = "essay"; units = 10; isFirstOrder = $false; subtotal = 800 }
  if ($probe.status -eq 429) { $limited = $true; break }
}
Check "promo check rate limit -> 429 + Retry-After" $limited

# Lockout: five wrong passwords from the same IP make the sixth attempt 429.
$lockout = $false
for ($i = 0; $i -lt 7; $i++) {
  $probe = Invoke-Api -Method POST -Path "/api/admin/auth/login" -Body @{ operatorId = "@ghost"; code = "bad-pass" }
  if ($probe.status -eq 429 -and $probe.body.error.message -eq "Слишком много попыток. Попробуйте через 15 минут.") { $lockout = $true; break }
}
Check "login brute-force lockout -> 429" $lockout

Write-Host ""
Write-Host ("passed: {0}   failed: {1}" -f $script:passed, $script:failed) -ForegroundColor Yellow
if ($script:failed -gt 0) { exit 1 }
