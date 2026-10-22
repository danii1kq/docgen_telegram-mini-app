// Confirms a pending order through the operator API (demo operator login),
// then waits for the builtin generator and prints the per-work result files.
const base = process.env.SRUNAL_API ?? 'http://127.0.0.1:8080';
const number = process.argv[2];
const password = process.argv[3] ?? 'Eshkere9113!xxx!';
if (!number) {
  console.error('usage: node e2e-confirm.mjs <order-number> [operator-password]');
  process.exit(1);
}

const login = await fetch(`${base}/api/admin/auth/login`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ operatorId: '@operator_almaty', password }),
});
const session = await login.json().catch(() => ({}));
console.log('login:', login.status, session.token ? 'token ok' : JSON.stringify(session).slice(0, 160));
if (!session.token) process.exit(1);

const auth = { 'Content-Type': 'application/json', Authorization: `Bearer ${session.token}` };
const confirm = await fetch(`${base}/api/admin/orders/${number}/confirm`, { method: 'POST', headers: auth, body: '{}' });
const confirmed = await confirm.json().catch(() => ({}));
console.log('confirm:', confirm.status, confirmed.status ?? JSON.stringify(confirmed).slice(0, 160));

const deadline = Date.now() + 180_000;
let last = '';
while (Date.now() < deadline) {
  const res = await fetch(`${base}/api/admin/orders/${number}`, { headers: auth });
  const order = await res.json();
  if (order.status !== last) {
    console.log('status:', order.status, 'cost:', order.agentCostUsd ?? 0);
    last = order.status;
  }
  if (order.status === 'completed' || order.status === 'failed_final') {
    const files = order.resultFiles ?? [];
    console.log('files:', files.length, files.map((f) => `${f.name} (${f.size} B)`).join(' | '));
    for (const file of files) {
      console.log(' url:', file.url?.slice(0, 90));
    }
    break;
  }
  await new Promise((r) => setTimeout(r, 3000));
}
