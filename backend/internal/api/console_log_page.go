package api

// consoleLogPage is the standalone generation-log viewer: the operator opens the
// signed link in any browser (no Telegram initData there). The page polls the
// JSON flavour of the same endpoint and appends only the new events, so it never
// reloads or jumps the scroll while the agent works.
//
// Placeholders (replaced with strings.ReplaceAll, not fmt, so the CSS/JS braces
// stay intact): __NUMBER__, __TOKEN__, __STATUS__.
const consoleLogPage = `<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Лог генерации __NUMBER__</title>
<style>
  :root { color-scheme: light dark; --bg:#ffffff; --ink:#111418; --ink2:#6b7280; --line:#e5e7eb; --accent:#2481cc; --tool:#f3f4f6; --warn:#b45309; --err:#b91c1c; --ok:#15803d; }
  @media (prefers-color-scheme: dark) { :root { --bg:#0e1621; --ink:#e8eaed; --ink2:#93a1af; --line:#22303c; --accent:#6ab3f3; --tool:#17212b; --warn:#d9a95c; --err:#fca5a5; --ok:#6ee7a0; } }
  * { box-sizing: border-box; }
  body { margin:0; background:var(--bg); color:var(--ink); font:14px/1.45 -apple-system, system-ui, "Segoe UI", Roboto, sans-serif; }
  header { position:sticky; top:0; display:flex; flex-wrap:wrap; gap:8px 16px; align-items:center; padding:10px 16px; background:color-mix(in srgb, var(--bg) 88%, transparent); backdrop-filter:blur(8px); border-bottom:1px solid var(--line); }
  .title { font-weight:600; }
  .meta { color:var(--ink2); font-size:12px; }
  .meta span { white-space:nowrap; }
  .actions { margin-left:auto; display:flex; gap:8px; }
  .actions a, .actions button { font:inherit; font-size:12px; color:var(--accent); background:transparent; border:1px solid var(--line); border-radius:8px; padding:4px 10px; cursor:pointer; text-decoration:none; }
  .actions button.on { border-color:var(--accent); }
  main { padding:8px 16px 48px; max-width:1100px; }
  .line { display:flex; gap:8px; padding:3px 0; border-bottom:1px dashed color-mix(in srgb, var(--line) 70%, transparent); font-family:ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size:12.5px; white-space:pre-wrap; word-break:break-word; }
  .line .at { color:var(--ink2); flex:0 0 auto; }
  .line.iteration { margin-top:12px; color:var(--accent); font-weight:600; border-bottom-color:var(--accent); font-family:inherit; font-size:13px; }
  .line.usage { color:var(--warn); }
  .line.error { color:var(--err); }
  .line.result { color:var(--ok); font-weight:600; }
  .line.event, .line.reasoning { color:var(--ink2); }
  .line.tool summary { cursor:pointer; color:var(--accent); }
  .line.tool pre { margin:6px 0 2px; padding:8px 10px; background:var(--tool); border-radius:8px; overflow:auto; max-height:320px; white-space:pre-wrap; }
  .empty { color:var(--ink2); padding:24px 0; }
</style>
</head>
<body>
<header>
  <div class="title">Лог генерации __NUMBER__</div>
  <div class="meta"><span id="status">__STATUS__</span><span id="cost"></span><span id="updated"></span></div>
  <div class="actions">
    <a href="?token=__TOKEN__&amp;download=1">Скачать raw</a>
    <button id="follow" class="on" type="button">Следить</button>
    <button id="bottom" type="button">Вниз</button>
  </div>
</header>
<main id="log"><div class="empty">Загрузка…</div></main>
<script>
(function () {
  var TOKEN = "__TOKEN__";
  var log = document.getElementById('log');
  var statusEl = document.getElementById('status');
  var costEl = document.getElementById('cost');
  var updatedEl = document.getElementById('updated');
  var rendered = [];
  var following = true;

  function lineKey(line) {
    return (line.at || '') + '|' + (line.text || '').slice(0, 80);
  }

  function renderLine(line) {
    var wrap = document.createElement('div');
    wrap.className = 'line ' + (line.kind || 'text');
    if (line.at) {
      var at = document.createElement('span');
      at.className = 'at';
      at.textContent = line.at;
      wrap.appendChild(at);
    }
    if (line.kind === 'tool') {
      var details = document.createElement('details');
      var summary = document.createElement('summary');
      var flat = (line.text || '').replace(/\s+/g, ' ');
      summary.textContent = flat.length > 140 ? flat.slice(0, 140) + ' …' : flat;
      details.appendChild(summary);
      var pre = document.createElement('pre');
      pre.textContent = line.text || '';
      details.appendChild(pre);
      wrap.appendChild(details);
    } else {
      var span = document.createElement('span');
      span.textContent = line.text || '';
      wrap.appendChild(span);
    }
    return wrap;
  }

  function scrollDown() {
    if (log.lastElementChild) log.lastElementChild.scrollIntoView({ block: 'end' });
  }

  function tick() {
    if (!following) return;
    fetch(location.pathname + '?token=' + encodeURIComponent(TOKEN) + '&json=1', { cache: 'no-store' })
      .then(function (res) { if (!res.ok) throw new Error('http ' + res.status); return res.json(); })
      .then(function (data) {
        statusEl.textContent = data.status + (data.jobStatus ? ' · ' + data.jobStatus : '');
        costEl.textContent = data.costUsd ? ' · $' + data.costUsd.toFixed(4) : '';
        updatedEl.textContent = data.updatedAt ? ' · ' + new Date(data.updatedAt).toLocaleTimeString() : '';
        var lines = data.lines || [];
        var keys = lines.map(lineKey);
        var same = rendered.length > 0 && keys.length >= rendered.length &&
          keys.slice(0, rendered.length).join('\u0000') === rendered.join('\u0000');
        if (!same) {
          log.replaceChildren.apply(log, lines.map(renderLine));
          rendered = keys;
          scrollDown();
          return;
        }
        for (var i = rendered.length; i < lines.length; i++) {
          log.appendChild(renderLine(lines[i]));
        }
        if (keys.length !== rendered.length) {
          rendered = keys;
          scrollDown();
        }
      })
      .catch(function () { /* next tick will retry */ });
  }

  document.getElementById('follow').addEventListener('click', function (event) {
    following = !following;
    event.target.classList.toggle('on', following);
    event.target.textContent = following ? 'Следить' : 'Пауза';
    if (following) tick();
  });
  document.getElementById('bottom').addEventListener('click', scrollDown);

  setInterval(tick, 3000);
  tick();
})();
</script>
</body>
</html>
`
