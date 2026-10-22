// i18n coverage check: lists `t('...')` keys used in the client screens that have
// no kk/en entry in src/i18n/dict.ts, plus Russian text left outside t().
//
// Usage: node scripts/i18n-check.mjs
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1');
const SRC = join(ROOT, 'src');
const SKIP_DIRS = ['operator', 'i18n'];
const SKIP_FILES = ['legal.ts'];

function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      if (SKIP_DIRS.includes(entry)) continue;
      out.push(...walk(full));
    } else if (/\.tsx?$/.test(entry) && !SKIP_FILES.includes(entry)) {
      out.push(full);
    }
  }
  return out;
}

const files = walk(SRC);
const dictText = readFileSync(join(SRC, 'i18n', 'dict.ts'), 'utf8');

const used = new Map(); // key -> [files]
const keyPattern = /\bt\(\s*'((?:[^'\\\n]|\\.)+)'/g;

for (const file of files) {
  const text = readFileSync(file, 'utf8');
  for (const match of text.matchAll(keyPattern)) {
    const key = match[1].replace(/\\'/g, "'");
    if (!/[А-Яа-яЁё]/.test(key)) continue; // non-Russian keys are not translated
    const list = used.get(key) ?? [];
    list.push(relative(ROOT, file));
    used.set(key, list);
  }
}

const missing = [...used.entries()]
  .filter(([key]) => !dictText.includes(`'${key}':`))
  .sort((a, b) => a[0].localeCompare(b[0], 'ru'));

console.log(`keys used: ${used.size}; missing translations: ${missing.length}`);
for (const [key, where] of missing) {
  console.log(`- ${key}   [${[...new Set(where)].join(', ')}]`);
}
