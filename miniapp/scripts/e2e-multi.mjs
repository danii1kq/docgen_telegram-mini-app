// Local end-to-end check of the multi-work flow:
//   create a 3-work order -> claim -> confirm -> wait for the builtin generator
//   -> print the per-work result files (one document per work).
const base = process.env.SRUNAL_API ?? 'http://127.0.0.1:8080';
const initData = process.argv[2] ?? '';
if (!initData) {
  console.error('usage: node e2e-multi.mjs "<initData>"');
  process.exit(1);
}

const headers = {
  'Content-Type': 'application/json',
  'X-Telegram-Init-Data': initData,
};

async function call(path, options = {}) {
  const res = await fetch(base + path, { ...options, headers: { ...headers, ...(options.headers ?? {}) } });
  const text = await res.text();
  let body = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = text;
  }
  return { status: res.status, body };
}

const me = await call('/api/auth/me');
console.log('me:', me.status, JSON.stringify(me.body));

const order = await call('/api/orders', {
  method: 'POST',
  headers: { 'Idempotency-Key': `e2e-multi-${Date.now()}` },
  body: JSON.stringify({
    product: 'essay',
    works: 3,
    units: 5,
    workKind: 'Реферат',
    topics: 'Тема первая: алгоритмы сортировки\nТема вторая: структуры данных\nТема третья: базы данных',
    notes: '',
    language: 'ru',
    institute: 'Институт автоматики и информационных технологий',
    department: 'Программной инженерии',
    studentName: 'Ануар Б.',
    disciplineCode: '6B07329',
    teacherName: 'Иванов И.',
    attachments: [],
    contactName: 'Ануар Б.',
    consent: true,
    promoCode: null,
  }),
});
console.log('create:', order.status, JSON.stringify(order.body).slice(0, 200));
const number = order.body?.number;
if (!number) process.exit(1);

const claim = await call(`/api/orders/${number}/claim`, {
  method: 'POST',
  body: JSON.stringify({ senderName: 'Ануар А.' }),
});
console.log('claim:', claim.status, claim.body?.status ?? JSON.stringify(claim.body).slice(0, 120));

const confirm = await call(`/api/admin/orders/${number}/confirm`, { method: 'POST', body: '{}' });
console.log('confirm:', confirm.status, confirm.body?.status ?? JSON.stringify(confirm.body).slice(0, 160));

const deadline = Date.now() + 180_000;
let last = '';
while (Date.now() < deadline) {
  const res = await call(`/api/orders/${number}`);
  const status = res.body?.status ?? '';
  if (status !== last) {
    console.log('status:', status);
    last = status;
  }
  if (status === 'completed' || status === 'failed_final') break;
  await new Promise((r) => setTimeout(r, 3000));
}

const final = await call(`/api/orders/${number}`);
const files = final.body?.resultFiles ?? [];
console.log('files:', files.length);
for (const file of files) {
  console.log(` - #${file.id} ${file.name} ${file.size} bytes`);
}
const event = (final.body?.events ?? []).slice(-1)[0];
console.log('last event:', event?.text ?? '-');
