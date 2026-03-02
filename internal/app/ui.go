package app

import "net/http"

func (d *Daemon) registerUIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", d.handleUIIndex)
}

func (d *Daemon) handleUIIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(uiHTML))
}

const uiHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Transcribe CLI UI</title>
  <style>
    :root { --bg:#081022; --card:#111d36; --fg:#edf2ff; --muted:#9fb0d4; --accent:#63d2ff; --ok:#7df8a3; --warn:#ffd27d; --err:#ff9c9c; }
    body { margin:0; font-family: "Segoe UI", -apple-system, sans-serif; background:radial-gradient(1200px 600px at 20% -20%, #1c2d53 0%, #081022 60%); color:var(--fg); }
    .wrap { max-width:1140px; margin:20px auto; padding:0 16px 32px; }
    h1 { margin:0 0 10px; font-size:28px; letter-spacing:0.2px; }
    h3 { margin:0 0 8px; }
    .grid { display:grid; grid-template-columns: repeat(auto-fit,minmax(320px,1fr)); gap:12px; }
    .card { background:var(--card); border:1px solid #223660; border-radius:14px; padding:12px; }
    .muted { color:var(--muted); }
    .row { display:flex; gap:8px; flex-wrap:wrap; align-items:center; margin:8px 0; }
    .stack > div { margin-bottom:6px; }
    input,select,button { border-radius:8px; border:1px solid #355083; padding:8px 10px; background:#0b1730; color:var(--fg); }
    button { cursor:pointer; background:#17305a; }
    button.primary { background:#2257a8; border-color:#3f76cb; }
    button.good { background:#1f5f42; border-color:#3ea66f; }
    button[disabled] { opacity:.5; cursor:not-allowed; }
    table { width:100%; border-collapse:collapse; font-size:13px; }
    th,td { text-align:left; border-bottom:1px solid #213156; padding:6px; vertical-align:top; }
    .status-ok { color:var(--ok); }
    .status-bad { color:var(--err); }
    .status-warn { color:var(--warn); }
    .pill { padding:2px 8px; border-radius:999px; background:#1a2c4e; font-size:12px; }
    .links a { color:var(--accent); margin-right:8px; }
    .boot-item { display:flex; justify-content:space-between; gap:8px; border:1px solid #243a66; border-radius:8px; padding:7px 9px; margin:6px 0; background:#0c1833; }
    code { word-break:break-all; }
  </style>
</head>
<body>
  <div class="wrap">
    <h1>Transcribe UI</h1>
    <div id="health" class="muted">Checking daemon...</div>

    <div class="grid" style="margin-top:12px;">
      <section class="card">
        <h3>Onboarding</h3>
        <div id="bootstrapSummary" class="muted">Checking runtime...</div>
        <div id="bootstrapList"></div>
      </section>

      <section class="card">
        <h3>App Update</h3>
        <div id="updateSummary" class="muted">Checking updates...</div>
      </section>
    </div>

    <div class="grid" style="margin-top:12px;">
      <section class="card">
        <h3>Runtime</h3>
        <div class="row"><span class="muted">Default model:</span> <span id="defaultModel" class="pill">-</span></div>
        <div class="row"><span class="muted">Models dir:</span> <code id="modelsDir">-</code></div>
      </section>

      <section class="card">
        <h3>Models</h3>
        <div class="row">
          <select id="modelSelect"></select>
          <button id="setDefaultBtn">Set Default</button>
        </div>
        <div class="row">
          <select id="presetSelect"></select>
          <button id="installPresetBtn" class="good">Download Preset</button>
        </div>
        <div id="modelMsg" class="muted"></div>
      </section>
    </div>

    <section class="card" style="margin-top:12px;">
      <h3>Transcribe File</h3>
      <div class="row">
        <input id="fileInput" type="file" accept="audio/*,video/*" />
      </div>
      <div class="row">
        <label>Language: <input id="langInput" value="auto" style="width:110px" /></label>
        <label>Model: <input id="runModelInput" value="ggml-base" style="width:160px" /></label>
        <button id="uploadBtn" class="primary">Start Transcription</button>
      </div>
      <div id="uploadMsg" class="muted"></div>
    </section>

    <section class="card" style="margin-top:12px;">
      <h3>Jobs</h3>
      <table>
        <thead><tr><th>ID</th><th>Status</th><th>Progress</th><th>Model</th><th>File</th><th>Results</th><th>Actions</th></tr></thead>
        <tbody id="jobsBody"></tbody>
      </table>
    </section>
  </div>

<script>
const state = {
  jobs: [],
  models: [],
  presets: [],
  defaultModel: 'ggml-base',
  modelsDir: '-',
  bootstrap: { ready: false, inProgress: false, components: [] },
  update: { enabled: false, message: '' },
};

async function api(path, options={}) {
  const res = await fetch(path, options);
  const text = await res.text();
  let body = null;
  try { body = text ? JSON.parse(text) : null; } catch { body = null; }
  if (!res.ok) {
    const msg = body && body.error ? body.error : ('HTTP ' + res.status);
    throw new Error(msg);
  }
  return body;
}

function setMsg(id, msg, ok=true) {
  const el = document.getElementById(id);
  el.textContent = msg;
  el.className = ok ? 'status-ok' : 'status-bad';
}

function lockControls(locked) {
  const ids = ['uploadBtn','setDefaultBtn','installPresetBtn','modelSelect','presetSelect','fileInput','langInput','runModelInput'];
  for (const id of ids) {
    const el = document.getElementById(id);
    if (el) el.disabled = !!locked;
  }
}

function refreshBootstrapUI() {
  const summary = document.getElementById('bootstrapSummary');
  const list = document.getElementById('bootstrapList');
  const s = state.bootstrap || {};

  list.innerHTML = '';
  for (const c of (s.components || [])) {
    const div = document.createElement('div');
    div.className = 'boot-item';
    const stateClass = c.status === 'ready' ? 'status-ok' : (c.status === 'installing' ? 'status-warn' : 'status-bad');
    const stateText = c.status || 'unknown';
    const msg = c.message ? ('<div class="muted">' + c.message + '</div>') : '';
    const path = c.path ? ('<div class="muted"><code>' + c.path + '</code></div>') : '';
    div.innerHTML = '<div><strong>' + c.name + '</strong>' + msg + path + '</div><div class="' + stateClass + '">' + stateText + '</div>';
    list.appendChild(div);
  }

  if (s.ready) {
    summary.textContent = 'Runtime is ready.';
    summary.className = 'status-ok';
  } else if (s.inProgress) {
    summary.textContent = 'Preparing runtime automatically...';
    summary.className = 'status-warn';
  } else if (s.error) {
    summary.textContent = 'Runtime setup failed: ' + s.error;
    summary.className = 'status-bad';
  } else {
    summary.textContent = 'Runtime setup is required, starting automatically...';
    summary.className = 'status-warn';
  }

  lockControls(!s.ready);
}

function refreshUpdateUI() {
  const el = document.getElementById('updateSummary');
  const up = state.update || {};

  if (!up.enabled) {
    el.textContent = 'Auto-update is disabled.';
    el.className = 'muted';
    return;
  }
  if (up.inProgress) {
    el.textContent = 'Checking for updates...';
    el.className = 'status-warn';
    return;
  }
  if (up.error) {
    el.textContent = 'Update check error: ' + up.error;
    el.className = 'status-bad';
    return;
  }
  const parts = [];
  if (up.currentVersion) parts.push('Current: ' + up.currentVersion);
  if (up.latestVersion) parts.push('Latest: ' + up.latestVersion);
  if (up.message) parts.push(up.message);
  el.textContent = parts.join(' | ') || 'No update information yet';
  el.className = up.updateAvailable ? 'status-warn' : 'status-ok';
}

function refreshModelsUI() {
  document.getElementById('defaultModel').textContent = state.defaultModel || '-';
  document.getElementById('modelsDir').textContent = state.modelsDir || '-';

  const modelSelect = document.getElementById('modelSelect');
  modelSelect.innerHTML = '';
  for (const m of state.models) {
    const o = document.createElement('option');
    o.value = m.name;
    o.textContent = m.name;
    if (m.name === state.defaultModel) o.selected = true;
    modelSelect.appendChild(o);
  }

  const runModel = document.getElementById('runModelInput');
  if (state.defaultModel && !runModel.value) runModel.value = state.defaultModel;

  const presetSelect = document.getElementById('presetSelect');
  presetSelect.innerHTML = '';
  for (const p of state.presets) {
    const o = document.createElement('option');
    o.value = p.name;
    o.textContent = p.alias + ' (' + p.name + ')';
    presetSelect.appendChild(o);
  }
}

function jobResultLinks(job) {
  const links = [];
  if (job.status === 'completed') {
    links.push('<a href="/v1/jobs/' + job.id + '/result/txt" target="_blank">txt</a>');
    links.push('<a href="/v1/jobs/' + job.id + '/result/srt" target="_blank">srt</a>');
    links.push('<a href="/v1/jobs/' + job.id + '/result/vtt" target="_blank">vtt</a>');
  }
  return links.join(' ');
}

function jobActions(job) {
  const out = [];
  if (['queued','preparing','transcoding','transcribing'].includes(job.status)) {
    out.push('<button data-cancel="' + job.id + '">Cancel</button>');
  }
  if (['failed','canceled'].includes(job.status)) {
    out.push('<button data-retry="' + job.id + '">Retry</button>');
  }
  return out.join(' ');
}

function refreshJobsUI() {
  const body = document.getElementById('jobsBody');
  body.innerHTML = '';
  for (const job of state.jobs) {
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td><code>' + job.id + '</code></td>' +
      '<td>' + job.status + '</td>' +
      '<td>' + job.progress + '%</td>' +
      '<td>' + (job.model || '-') + '</td>' +
      '<td title="' + job.filePath + '">' + job.filePath + '</td>' +
      '<td class="links">' + jobResultLinks(job) + '</td>' +
      '<td>' + jobActions(job) + '</td>';
    body.appendChild(tr);
  }
}

async function refreshData() {
  try {
    await api('/healthz');
    const health = document.getElementById('health');
    health.textContent = 'Daemon healthy';
    health.className = 'status-ok';

    state.bootstrap = await api('/v1/bootstrap/status');
    if (!state.bootstrap.ready && !state.bootstrap.inProgress) {
      await api('/v1/bootstrap/ensure', { method: 'POST' });
      state.bootstrap = await api('/v1/bootstrap/status');
    }
    refreshBootstrapUI();

    state.update = await api('/v1/update/status');
    refreshUpdateUI();

    const modelData = await api('/v1/models');
    state.models = modelData.models || [];
    state.defaultModel = modelData.defaultModel || 'ggml-base';
    state.modelsDir = modelData.modelsDir || '-';

    const presets = await api('/v1/models/presets');
    state.presets = presets.presets || [];

    const jobs = await api('/v1/jobs');
    state.jobs = jobs.jobs || [];

    refreshModelsUI();
    refreshJobsUI();
  } catch (err) {
    const health = document.getElementById('health');
    health.textContent = 'Error: ' + err.message;
    health.className = 'status-bad';
  }
}

document.getElementById('setDefaultBtn').addEventListener('click', async () => {
  const name = document.getElementById('modelSelect').value;
  if (!name) return;
  try {
    await api('/v1/models/use', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name }) });
    setMsg('modelMsg', 'Default model updated: ' + name, true);
    await refreshData();
  } catch (err) {
    setMsg('modelMsg', err.message, false);
  }
});

document.getElementById('installPresetBtn').addEventListener('click', async () => {
  const name = document.getElementById('presetSelect').value;
  if (!name) return;
  try {
    setMsg('modelMsg', 'Downloading ' + name + ' ...', true);
    await api('/v1/models/install', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name }) });
    setMsg('modelMsg', 'Installed: ' + name, true);
    await refreshData();
  } catch (err) {
    setMsg('modelMsg', err.message, false);
  }
});

document.getElementById('uploadBtn').addEventListener('click', async () => {
  if (!state.bootstrap.ready) {
    setMsg('uploadMsg', 'Runtime is still preparing, please wait.', false);
    return;
  }

  const fileInput = document.getElementById('fileInput');
  const file = fileInput.files && fileInput.files[0];
  if (!file) {
    setMsg('uploadMsg', 'Please select an audio/video file', false);
    return;
  }

  const form = new FormData();
  form.append('file', file);
  form.append('language', document.getElementById('langInput').value || 'auto');
  form.append('model', document.getElementById('runModelInput').value || state.defaultModel || 'ggml-base');

  try {
    setMsg('uploadMsg', 'Uploading and queueing...', true);
    const job = await api('/v1/jobs/upload', { method: 'POST', body: form });
    setMsg('uploadMsg', 'Queued: ' + job.id, true);
    fileInput.value = '';
    await refreshData();
  } catch (err) {
    setMsg('uploadMsg', err.message, false);
  }
});

document.getElementById('jobsBody').addEventListener('click', async (e) => {
  const cancelId = e.target.getAttribute('data-cancel');
  const retryId = e.target.getAttribute('data-retry');
  try {
    if (cancelId) {
      await api('/v1/jobs/' + cancelId + '/cancel', { method: 'POST' });
      await refreshData();
      return;
    }
    if (retryId) {
      await api('/v1/jobs/' + retryId + '/retry', { method: 'POST' });
      await refreshData();
      return;
    }
  } catch (err) {
    alert(err.message);
  }
});

refreshData();
setInterval(refreshData, 2000);
</script>
</body>
</html>`
