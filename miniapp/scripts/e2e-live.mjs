#!/usr/bin/env node
/**
 * Live end-to-end check of the client flow against a running backend and a
 * REAL Telegram chat, with the bot enabled:
 *
 *   order created → «Я оплатил» → operator confirm → generation → result file
 *
 *   node scripts/e2e-live.mjs [chatId] [--confirm] [--password <pw>]
 *
 * The chat must be a Telegram chat that started the bot (its notifications are
 * the point of the check); `chatId` defaults to the bound operator chat.
 * initData is signed locally with the bot token from ../backend/.env.
 *
 * Without --confirm the run stops after «Я оплатил» and prints the order number,
 * so the operator buttons can be pressed in Telegram by hand.
 */

import { createHmac } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const baseUrl = process.env.SRUNAL_API ?? 'http://127.0.0.1:8080';

const args = process.argv.slice(2);
const flagIndex = (name) => args.indexOf(name);
const passwordIndex = flagIndex('--password');
const password = passwordIndex >= 0 ? args[passwordIndex + 1] : 'Eshkere9113!xxx!';
const shouldConfirm = flagIndex('--confirm') >= 0;
// The chat id of the client is a personal identifier: it is always an argument
// and never a default value in the repository.
const chatIdArg = args.find((value, index) => /^\d+$/.test(value) && !args[index - 1]?.startsWith('--'));
if (!chatIdArg) {
  console.error('usage: node e2e-live.mjs <client-chat-id> [--password <password>] [--confirm]');
  process.exit(1);
}
const chatId = Number(chatIdArg);

function readEnvFile() {
  const raw = readFileSync(resolve(here, '../../backend/.env'), 'utf8');
  const env = {};
  for (const line of raw.split(/\r?\n/)) {
    const match = /^\s*([A-Z0-9_]+)\s*=\s*(.*)\s*$/.exec(line);
    if (match) env[match[1]] = match[2].trim();
  }
  return env;
}

function signInitData(token, userId, username) {
  const user = JSON.stringify({
    id: userId,
    first_name: 'Даниил',
    last_name: 'Л.',
    username,
    language_code: 'ru',
  });
  const fields = { auth_date: String(Math.floor(Date.now() / 1000)), query_id: `AA${Date.now()}`, user };
  const dataCheckString = Object.keys(fields)
    .sort()
    .map((key) => `${key}=${fields[key]}`)
    .join('\n');
  const secretKey = createHmac('sha256', 'WebAppData').update(token).digest();
  const hash = createHmac('sha256', secretKey).update(dataCheckString).digest('hex');
  return new URLSearchParams({ ...fields, hash }).toString();
}

const env = readEnvFile();
const initData = signInitData(env.TELEGRAM_BOT_TOKEN, chatId, 'owner');

async function call(path, options = {}) {
  const response = await fetch(`${baseUrl}${path}`, {
    ...options,
    headers: {
      Accept: 'application/json',
      'X-Telegram-Init-Data': initData,
      ...(options.body ? { 'Content-Type': 'application/json' } : {}),
      ...(options.headers ?? {}),
    },
  });
  const text = await response.text();
  let payload = null;
  try {
    payload = JSON.parse(text);
  } catch {
    payload = text;
  }
  if (!response.ok) {
    throw new Error(`${options.method ?? 'GET'} ${path} → ${response.status}: ${text.slice(0, 300)}`);
  }
  return payload;
}

const sleep = (ms) => new Promise((done) => setTimeout(done, ms));

console.log(`chat ${chatId} · api ${baseUrl}`);

const me = await call('/api/auth/me');
console.log(`1. session        · chatId=${me.chatId} username=@${me.username}`);

const created = await call('/api/orders', {
  method: 'POST',
  headers: { 'Idempotency-Key': `e2e-${Date.now()}` },
  body: JSON.stringify({
    product: 'essay',
    works: 1,
    units: 10,
    workKind: '',
    topics: 'E2E-проверка: оплата, генерация и доставка файла в чат',
    notes: '',
    language: 'ru',
    institute: 'ИЦИТ',
    department: 'Кафедра компьютерных наук',
    studentName: 'Даниил Л.',
    disciplineCode: 'CS101',
    teacherName: 'Ержан Мырзабеков',
    attachments: [],
    contactName: 'Даниил Л.',
    consent: true,
    promoCode: null,
  }),
});
console.log(`2. order created  · ${created.number} · ${created.amount} ₸ · ${created.status}`);

const claimed = await call(`/api/orders/${created.number}/claim`, {
  method: 'POST',
  body: JSON.stringify({ senderName: 'Даниил Л.' }),
});
console.log(`3. «Я оплатил»    · status=${claimed.status} (уведомление оператору с кнопками)`);

if (!shouldConfirm) {
  console.log('\nОстановились до подтверждения: нажмите ✅/❌ в чате оператора или перезапустите с --confirm.');
  console.log(`Заказ: ${created.number}`);
  process.exit(0);
}

const login = await fetch(`${baseUrl}/api/admin/auth/login`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ operatorId: '@operator_almaty', password }),
});
if (!login.ok) {
  throw new Error(`operator login failed: ${login.status} ${await login.text()}`);
}
const { token } = await login.json();

const confirm = await fetch(`${baseUrl}/api/admin/orders/${created.number}/confirm`, {
  method: 'POST',
  headers: { Authorization: `Bearer ${token}` },
});
if (!confirm.ok) {
  throw new Error(`confirm failed: ${confirm.status} ${await confirm.text()}`);
}
console.log('4. operator       · оплата подтверждена, заказ в очереди');

// A real agent run on the pinned model takes 5–15 minutes; keep watching for
// half an hour before giving up.
for (let attempt = 1; attempt <= 360; attempt += 1) {
  await sleep(5000);
  const order = await call(`/api/orders/${created.number}`);
  if (order.status === 'completed') {
    console.log(`5. generation     · completed за ${attempt * 5} с`);
    console.log(`6. result         · ${order.resultFile?.name} (${order.resultFile?.size} байт)`);
    console.log(`   download       · ${order.resultFile?.url}`);
    console.log('\nПроверьте чат: файл должен прийти сообщением с подписью «🎓 Работа готова!».');
    process.exit(0);
  }
  if (order.status === 'failed_final') {
    console.error(`5. generation     · FAILED (${order.paymentRejectedReason ?? 'см. журнал воркера'})`);
    process.exit(1);
  }
  process.stdout.write(`\r5. generation     · ${order.status} (${attempt * 5} с)   `);
}
console.error('\ngeneration did not finish in 30 minutes');
process.exit(1);
