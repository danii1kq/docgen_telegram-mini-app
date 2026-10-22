#!/usr/bin/env node
/**
 * Mini App API smoke test: drives the chat-identity flow against a running
 * backend with a locally signed initData payload (no Telegram needed).
 *
 *   node scripts/smoke-api.mjs [baseUrl]
 *
 * Checks: /auth/me, /me/orders, /quote, order creation, «Я оплатил» by chat
 * (no order token), reading the own order, and the neutral 404 for a foreign
 * order.
 */

import { createHmac } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const baseUrl = process.argv[2] ?? 'http://127.0.0.1:8080';

function readToken() {
  if (process.env.TELEGRAM_BOT_TOKEN) return process.env.TELEGRAM_BOT_TOKEN.trim();
  const raw = readFileSync(resolve(here, '../../backend/.env'), 'utf8');
  for (const line of raw.split(/\r?\n/)) {
    const match = /^\s*TELEGRAM_BOT_TOKEN\s*=\s*(.+)\s*$/.exec(line);
    if (match && match[1].trim()) return match[1].trim();
  }
  throw new Error('TELEGRAM_BOT_TOKEN not found (environment or backend/.env)');
}

function signInitData(token, userId, username) {
  const user = JSON.stringify({
    id: userId,
    first_name: 'Ануар',
    last_name: 'Бекболатов',
    username,
    language_code: 'ru',
  });
  const fields = {
    auth_date: String(Math.floor(Date.now() / 1000)),
    query_id: `AA${Date.now()}`,
    user,
  };
  const dataCheckString = Object.keys(fields)
    .sort()
    .map((key) => `${key}=${fields[key]}`)
    .join('\n');
  const secretKey = createHmac('sha256', 'WebAppData').update(token).digest();
  const hash = createHmac('sha256', secretKey).update(dataCheckString).digest('hex');
  return new URLSearchParams({ ...fields, hash }).toString();
}

const token = readToken();
const initData = signInitData(token, 555000777, 'srunal_smoke');

let failures = 0;
function check(name, ok, detail = '') {
  const mark = ok ? 'ok  ' : 'FAIL';
  console.log(`${mark} ${name}${detail ? ` — ${detail}` : ''}`);
  if (!ok) failures += 1;
}

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
  return { status: response.status, payload };
}

const me = await call('/api/auth/me');
check('GET /api/auth/me', me.status === 200 && me.payload?.chatId === 555000777);

const quote = await call('/api/quote', {
  method: 'POST',
  body: JSON.stringify({ product: 'essay', units: 10, works: 1 }),
});
check('POST /api/quote', quote.status === 200 && quote.payload?.total === 800, `total=${quote.payload?.total}`);

const created = await call('/api/orders', {
  method: 'POST',
  headers: { 'Idempotency-Key': `smoke-${Date.now()}` },
  body: JSON.stringify({
    product: 'essay',
    works: 1,
    units: 10,
    workKind: 'СРС',
    topics: 'Смоук-тест Mini App: сравнение алгоритмов сортировки',
    notes: '',
    language: 'ru',
    institute: 'ИЦИТ',
    department: 'Программная инженерия',
    studentName: 'Ануар Бекболатов',
    disciplineCode: 'CS101',
    teacherName: 'Ержан Мырзабеков',
    attachments: [],
    contactName: 'Ануар Бекболатов',
    consent: true,
    promoCode: null,
  }),
});
check('POST /api/orders', created.status === 201 && Boolean(created.payload?.number), created.payload?.number);
const number = created.payload?.number;

const claim = await call(`/api/orders/${number}/claim`, {
  method: 'POST',
  body: JSON.stringify({ senderName: 'Ануар А.' }),
});
check(
  'POST /api/orders/{number}/claim without token (chat identity)',
  claim.status === 200 && claim.payload?.status === 'payment_claimed',
  `status=${claim.payload?.status}`,
);

const own = await call(`/api/orders/${number}`);
check('GET /api/orders/{number} by initData chat', own.status === 200 && own.payload?.number === number);

const foreign = await call(`/api/orders/DG-260391`);
check(
  'a foreign order is not readable (neutral 404)',
  foreign.status === 404,
  `status=${foreign.status}`,
);

const list = await call('/api/me/orders');
const listed = (list.payload?.orders ?? []).some((order) => order.number === number);
check('GET /api/me/orders lists the chat orders', list.status === 200 && listed);

console.log(failures === 0 ? '\nAll checks passed.' : `\n${failures} check(s) failed.`);
process.exit(failures === 0 ? 0 : 1);
