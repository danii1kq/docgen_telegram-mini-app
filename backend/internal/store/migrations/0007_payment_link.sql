-- Payment link (Kaspi QR deep link).
--
-- The client payment block offers a red «Оплатить через Kaspi» button that opens
-- this link (qr.kaspi.kz), which hands the payment off to the Kaspi app. It is
-- an operator setting, like the uploaded QR image; when it is empty the Mini App
-- falls back to the QR plate.

alter table app_settings
  add column if not exists payment_link text not null default '';
