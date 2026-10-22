# Downloads the Stitch design assets (screenshots + HTML + docs) for the
# "Telegram Mini App Design System" project into design/stitch/screens.
# Public lh3/usercontent URLs need no key; stitch.googleapis.com ones do.
param(
  [string]$Key = $env:STITCH_API_KEY
)

$ErrorActionPreference = "Continue"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$out = Join-Path $root "screens"
New-Item -ItemType Directory -Force -Path $out | Out-Null

function Get-Asset([string]$Url, [string]$Path, [bool]$WithKey) {
  if (Test-Path $Path) { Remove-Item $Path -Force }
  if ($WithKey) {
    curl.exe -sSL --fail -H "X-Goog-Api-Key: $Key" -o "$Path" "$Url"
  } else {
    curl.exe -sSL --fail -o "$Path" "$Url"
  }
  if (Test-Path $Path) {
    $size = (Get-Item $Path).Length
    Write-Host ("{0,-34} {1,10:N0} bytes" -f (Split-Path $Path -Leaf), $size)
  } else {
    Write-Host ("{0,-34} FAILED" -f (Split-Path $Path -Leaf))
  }
}

$stitch = "https://stitch.googleapis.com/v1/projects/15543765733870549702/files"

# 01 — Главная · Калькулятор (Dark)
Get-Asset "https://lh3.googleusercontent.com/aida/AEtjO1XZ1mn4K_5YeXUzF15-VcPFAtF15jGYvLXV2r260BGMMH83yNrjRTlaKWLPtLt4rCsCz0saiYD2HuTp4xDj5GXWRyEbZpkODtPQYG8fijEmJCmVhoudhhFwo1Osqv7TD7DDB15vXAyuMQibY9ABvKVg7tNmEO8CERyUJz23jq1gHwRBYVIRET1taFLXhNK43m67pIxh1FfHBLMaqcU2NSHuYVJlBEr9f8A0HsSoqMP90KFoa5oMoEoWhHGn" (Join-Path $out "01-home-dark.png") $false
Get-Asset "https://contribution.usercontent.google.com/download?c=CgthaWRhX2NvZGVmeBJ8Eh1hcHBfY29tcGFuaW9uX2dlbmVyYXRlZF9maWxlcxpbCiVodG1sXzAwMDY1ZTA3NjgyYzRiM2EwOTY4ODA4ODVkMDE0ZmEzEgsSBxDzt8K5pgYYAZIBJAoKcHJvamVjdF9pZBIWQhQxNTU0Mzc2NTczMzg3MDU0OTcwMg&filename=&opi=89354086" (Join-Path $out "01-home-dark.html") $false

# 02 — Главная · Калькулятор
Get-Asset "$stitch/9121362959102579350:download" (Join-Path $out "02-home.png") $true
Get-Asset "$stitch/10963164149675742342:download" (Join-Path $out "02-home.html") $true

# 03 — Новый заказ · Шаг 1 (Dark)
Get-Asset "https://lh3.googleusercontent.com/aida/AEtjO1UPbmPUzI0e-pcZD6glLeIw41ggcYFuoNPQyvablxQ0DAONZA0bu2oWMMT1Jcv_uGknRqVDItzZG3_wQZx7i-zhGAJ-RBIRyhvuamRkdyPRtTbp6M7L8igmWcTUGEYeUCoWXoK5g81O_RS6v_CFFYAQKQQc8WTytj7gaJmSjYm-M0GEciZi7LQypQHZBDO0a5eHGs8dHF9_r6xNDb7NSrm-EKN36fbIyqnNgWONuwuNLD7Mfb2IJZ2zdl6w" (Join-Path $out "03-order-step1-dark.png") $false
Get-Asset "https://contribution.usercontent.google.com/download?c=CgthaWRhX2NvZGVmeBJ8Eh1hcHBfY29tcGFuaW9uX2dlbmVyYXRlZF9maWxlcxpbCiVodG1sXzAwMDY1ZTA3Njg2Y2M4N2IwNWYxM2Y3Zjg4MzZlYTM0EgsSBxDzt8K5pgYYAZIBJAoKcHJvamVjdF9pZBIWQhQxNTU0Mzc2NTczMzg3MDU0OTcwMg&filename=&opi=89354086" (Join-Path $out "03-order-step1-dark.html") $false

# 04 — Новый заказ · Шаг 1
Get-Asset "https://lh3.googleusercontent.com/aida/AEtjO1XVkfvcDJLXF1MNQGFAs58y1MJAyVk0812QrwEYUSCijNMHGNEfN2L_TvQGvz6PITPckH8TmyUweEba8Srl8j7qWtl2UpO9n6R3FKiqM5M49Z2f8EMFCwXUfQrq9VJpawpwnm3dcwW81phB9AVz9JKMprsO4MmW9lDP5-KkCkSFiIobJVLHqJT1ymxAxQg8jr2QPNJz4Fx6kncgIihpKeEcb1sxs-PejuyIs4zxPTeAJd0XkCJl6j5ew7yH" (Join-Path $out "04-order-step1.png") $false
Get-Asset "https://contribution.usercontent.google.com/download?c=CgthaWRhX2NvZGVmeBJ8Eh1hcHBfY29tcGFuaW9uX2dlbmVyYXRlZF9maWxlcxpbCiVodG1sXzAwMDY1ZTA3M2QyNGZkNTQwMjNiYzA0YmQyMTNhZWMzEgsSBxDzt8K5pgYYAZIBJAoKcHJvamVjdF9pZBIWQhQxNTU0Mzc2NTczMzg3MDU0OTcwMg&filename=&opi=89354086" (Join-Path $out "04-order-step1.html") $false

# 05 — Новый заказ · Шаг 2
Get-Asset "https://lh3.googleusercontent.com/aida/AEtjO1X0IHS2d1E5lDi0Kltvpj42zeYY8Rtq_4Eu9jfox-dmNNmlgubrhzbe6Fgj4LIBUA_qsc9tvlK1jZea7x6dNUoyOXGl0Q3PcKlLIfk9eCkz3WjhgdHzvk9z3wwEVeWkuCpEPvkKcW2Pz681gQfPLR4ICJyRTc2OYYHsEyGypgzqDK1lo8j1CYCoHSYH2J6pGdnIH2pPnpKi8Hjl7M_DQw_VqCYLi9_BiVt-uatUqau6UJeo6fj8sMM1hUWZ" (Join-Path $out "05-order-step2.png") $false
Get-Asset "https://contribution.usercontent.google.com/download?c=CgthaWRhX2NvZGVmeBJ8Eh1hcHBfY29tcGFuaW9uX2dlbmVyYXRlZF9maWxlcxpbCiVodG1sXzAwMDY1ZTA3M2MzZjQ1ZjIwMjNiZTkxOTNmMWVkMjZkEgsSBxDzt8K5pgYYAZIBJAoKcHJvamVjdF9pZBIWQhQxNTU0Mzc2NTczMzg3MDU0OTcwMg&filename=&opi=89354086" (Join-Path $out "05-order-step2.html") $false

# 06 — Новый заказ · Шаг 3
Get-Asset "https://lh3.googleusercontent.com/aida/AEtjO1VtHxTwKi48cO2vpT6Paar8a7if9Z3Sxl10ZfLL4aw63AEr9FV4teCydBJaChZYMTDlTml9jwbHBc9YvjaIW3_U6Fcsms39iHjZSsIM_1IF0BBxEpUqTxnJGj3lgy55HrBRgogAus7Af9W0xGWb6mvcZsCskZeNBoYHLX0E_Ft8JoYzbfpFzMwsq4w7fDU3FxLmjUnd5I8O0ZZeOHEeCZHkPQPlWAuNy0YcOxP7ujBLVjnZ9bM8K5z4vdA" (Join-Path $out "06-order-step3.png") $false
Get-Asset "https://contribution.usercontent.google.com/download?c=CgthaWRhX2NvZGVmeBJ8Eh1hcHBfY29tcGFuaW9uX2dlbmVyYXRlZF9maWxlcxpbCiVodG1sXzAwMDY1ZTA3NDdlNGUxYmQwMzU2YzY4YmJkMDRjOWQ3EgsSBxDzt8K5pgYYAZIBJAoKcHJvamVjdF9pZBIWQhQxNTU0Mzc2NTczMzg3MDU0OTcwMg&filename=&opi=89354086" (Join-Path $out "06-order-step3.html") $false

# 07 — Новый заказ · Шаг 4
Get-Asset "https://lh3.googleusercontent.com/aida/AEtjO1W0sKu8VVaKlBTaDjurPDTPSlAe7npmk2KQ1Xskom8H3dhC9LrBt171_3byazsRePh15Q5PlbYYtJn-Gr-8iX1HPzxPKyXGjbozN_SuueMS9atFMEqSzlHgveVMfplEW-osMXw5OqQngKMpawXPJU91HYpXXi-DPns17Bd-W0rVSw8cd50WqdWkIEk6YzT7gG_xuMWHoSpgGkftZz0ZHNIcbKtdCMxsywckKNa6y7sDGNdn8GlijlIK4YQ-" (Join-Path $out "07-order-step4.png") $false
Get-Asset "https://contribution.usercontent.google.com/download?c=CgthaWRhX2NvZGVmeBJ8Eh1hcHBfY29tcGFuaW9uX2dlbmVyYXRlZF9maWxlcxpbCiVodG1sXzAwMDY1ZTA3NWRiZjU1NDIwNWYxM2Y3Zjg4MzZlYTM0EgsSBxDzt8K5pgYYAZIBJAoKcHJvamVjdF9pZBIWQhQxNTU0Mzc2NTczMzg3MDU0OTcwMg&filename=&opi=89354086" (Join-Path $out "07-order-step4.html") $false

# 08 — Заказ · Оплата
Get-Asset "https://lh3.googleusercontent.com/aida/AEtjO1VTIb6p5asfiiBpw0NuHxJ1mkRlhoRtI0LLBXWT4awYCTDzDaIVNh5dhko3xongFX59orkJQ06jch50hRnRXJvz2pD6FoOZtvk4b5QgNP7R77JNgVcvt90biBmIcVbja6wYmHO3XfbDzJTHSAQO6cvPP5QJSW2ugJnNR-r9PDkX7kWIlwOISYP3mYg1KBTDpIDfxnAJECI9Tx1ZAddFfA23yHFrqRNynANVW64rT1yFnKhV_634EbmUExVb" (Join-Path $out "08-order-payment.png") $false
Get-Asset "https://contribution.usercontent.google.com/download?c=CgthaWRhX2NvZGVmeBJ8Eh1hcHBfY29tcGFuaW9uX2dlbmVyYXRlZF9maWxlcxpbCiVodG1sXzAwMDY1ZTA3ODYxZDFmNzIwOTY4ODA4ODVkMDE0ZmEzEgsSBxDzt8K5pgYYAZIBJAoKcHJvamVjdF9pZBIWQhQxNTU0Mzc2NTczMzg3MDU0OTcwMg&filename=&opi=89354086" (Join-Path $out "08-order-payment.html") $false

# 09 — Заказ · Оплата (Dark)
Get-Asset "https://lh3.googleusercontent.com/aida/AEtjO1WiA2hM5hN3nfETQiiBnFS9ZsTfnvDeC23awotDa7JNqqZXSEFBf4wuFF-ANXu5eJMBAzG0QfvJsiNJSMOeVOPfU4iWokb51VSvtQvflNUArEU9trfiN0f9RBEriDTLxtpg4EHW47nQLZTAeXhyr9t2-FHiEPpK6406k-dirTrGoytZpgnJ5SdKXHHQECkI86qIsVUeeHXHwPEW2gaiHYcELDkHc7Dse225dw8S2GDs79htkzJ2NpHBiMLE" (Join-Path $out "09-order-payment-dark.png") $false
Get-Asset "https://contribution.usercontent.google.com/download?c=CgthaWRhX2NvZGVmeBJ8Eh1hcHBfY29tcGFuaW9uX2dlbmVyYXRlZF9maWxlcxpbCiVodG1sXzAwMDY1ZTA3ODY3ZDE5MjMwMjNiZjQyMDdlMTM1NDc3EgsSBxDzt8K5pgYYAZIBJAoKcHJvamVjdF9pZBIWQhQxNTU0Mzc2NTczMzg3MDU0OTcwMg&filename=&opi=89354086" (Join-Path $out "09-order-payment-dark.html") $false

# 10 — Заказ · Готово
Get-Asset "$stitch/screenshot_00065dfc3dcad8f90479c941a0141faa:download" (Join-Path $out "10-order-done.png") $true
Get-Asset "$stitch/7242b0cee66f42f388a628f8034ed16b:download" (Join-Path $out "10-order-done.html") $true

# 11 — Мои заказы
Get-Asset "$stitch/screenshot_00065dfc3d9702ff045adde239022a79:download" (Join-Path $out "11-my-orders.png") $true
Get-Asset "$stitch/f1ac340c4ca6429ca685aecb0a931189:download" (Join-Path $out "11-my-orders.html") $true

# Project docs + logo assets
Get-Asset "$stitch/15543765733870547351:download" (Join-Path $out "design.md") $true
Get-Asset "$stitch/15543765733870550779:download" (Join-Path $out "prompt.md") $true
Get-Asset "https://lh3.googleusercontent.com/aida/AEtjO1UcJdilP7Lyyo9eL3p-gx06vIdlmdXkr9tcHlD3zjByB5xehndlFsDHfqI_hKobnK4IpXIre3ggaG6Udq1fhvIs47Fu6tFMkyeZShv3zHR9Mw_6yuKHyltTUm7zJsLjOh9sILb8p0suro7zvJwDHc3RiD0MXcBNFFGBaoh4POU9UUqVBoKaH86fz2Imxsm__B9ozOGxfugPW2TB4oVB2W5BTqhfHz-fhPJ_ZxGTcxHYTHRluLO0XgDNFTH0" (Join-Path $out "asset-logo.png") $false
Get-Asset "https://lh3.googleusercontent.com/aida/AEtjO1VgGFwoTAXtkhR6N8I-pSXhpV9oF8bBc2BxDET87y_cbYAJKPEMjvIPPjGV2di4AQXpdUBbq_jpJbo9QWjK5cv1JkyqT1YIP5Ezr3wZv4aF21OlZM5ne9Jj_T1ZKkIpGG9r9K9RBJMl1htaZsZ0FZE4zKKSa6WfyzGJZEevziEuhhQySQaRlTDZqyTSzkygR4X-9kb-43-iPfBBQKbrEZt4DukLCevYSKUhtKyykoaDCgCaIxOkc656SZc" (Join-Path $out "asset-wordmark.png") $false
