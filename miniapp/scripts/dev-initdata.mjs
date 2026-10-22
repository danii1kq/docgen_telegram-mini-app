#!/usr/bin/env node
/**
 * Dev harness: prints a signed `initData` string for the local backend, so the
 * Mini App can be exercised in a plain browser (`npm run dev`) as if it ran
 * inside Telegram.
 *
 *   node scripts/dev-initdata.mjs [userId] [username]
 *
 * The bot token is read from ../backend/.env (TELEGRAM_BOT_TOKEN) or the
 * TELEGRAM_BOT_TOKEN environment variable. Then, in the browser console:
 *
 *   localStorage.setItem('srunal.dev.initData', '<printed string>');
 *   location.reload();
 *
 * Signature scheme (core.telegram.org/bots/webapps):
 *   secret = HMAC_SHA256(key="WebAppData", data=bot_token)
 *   hash   = hex(HMAC_SHA256(key=secret, data=data_check_string))
 */

import { createHmac } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));

function readToken() {
  if (process.env.TELEGRAM_BOT_TOKEN) return process.env.TELEGRAM_BOT_TOKEN.trim();
  const envPath = resolve(here, '../../backend/.env');
  try {
    const raw = readFileSync(envPath, 'utf8');
    for (const line of raw.split(/\r?\n/)) {
      const match = /^\s*TELEGRAM_BOT_TOKEN\s*=\s*(.+)\s*$/.exec(line);
      if (match && match[1].trim()) return match[1].trim();
    }
  } catch {
    /* fall through */
  }
  console.error('TELEGRAM_BOT_TOKEN not found (environment or backend/.env)');
  process.exit(1);
}

const token = readToken();
const userId = process.argv[2] ?? '555000111';
const username = process.argv[3] ?? 'anuarbek';

const user = JSON.stringify({
  id: Number(userId),
  first_name: 'Ануар',
  last_name: 'Бекболатов',
  username,
  language_code: 'ru',
  allows_write_to_pm: true,
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

const initData = new URLSearchParams({ ...fields, hash }).toString();
console.log(initData);
