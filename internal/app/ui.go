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
    :root { --bg:#0b1020; --card:#131b31; --fg:#edf2ff; --muted:#9fb0d4; --accent:#63d2ff; --ok:#7df8a3; --warn:#ffd27d; --err:#ff9c9c; }
    body { margin:0; font-family: ui-sans-serif,system-ui,-apple-system,Segoe UI,Roboto,Arial; background:linear-gradient(180deg,#0b1020,#0f1730); color:var(--fg); }
    .wrap { max-width:1100px; margin:24px auto; padding:0 16px 32px; }
    h1 { margin:0 0 16px; font-size:28px; }
    .grid { display:grid; grid-template-columns: repeat(auto-fit,minmax(300px,1fr)); gap:12px; }
    .card { background:var(--card); border:1px solid #1d2a4a; border-radius:12px; padding:12px; }
    .muted { color:var(--muted); }
    .row { display:flex; gap:8px; flex-wrap:wrap; align-items:center; margin:8px 0; }
    input,select,button { border-radius:8px; border:1px solid #2c3d67; padding:8px 10px; background:#0f1730; color:var(--fg); }
    button { cursor:pointer; background:#1a2747; }
    button.primary { background:#2257a8; border-color:#3f76cb; }
    button.good { background:#1f5f42; border-color:#3ea66f; }
    table { width:100%; border-collapse:collapse; font-size:13px; }
    th,td { text-align:left; border-bottom:1px solid #213156; padding:6px; vertical-align:top; }
    .status-ok { color:var(--ok); }
    .status-bad { color:var(--err); }
    .pill { padding:2px 8px; border-radius:999px; background:#182541; font-size:12px; }
    .links a { color:var(--accent); margin-right:8px; }
  </style>
</head>
<body>
  <div class="wrap">
    <h1>Transcribe UI</h1>
    <div class="muted" id="health">Checking daemon...</div>

    <div class="grid" style="margin-top:12px;">
      <section class="card">
        <h3 style="margin:0 0 8px;">Runtime</h3>
        <div class="row"><span class="muted">Default model:</span> <span id="defaultModel" class="pill">-</span></div>
        <div class="row"><span class="muted">Models dir:</span> <code id="modelsDir">-</code></div>
      </section>

      <section class="card">
        <h3 style="margin:0 0 8px;">Models</h3>
        <div class="row">
          <select id="modelSelect"></select>
          <button id="setDefaultBtn">Set Default</button>
        </div>
        <div class="row">
          <select id="presetSelect"></select>
          <button id="installPresetBtn" class="good">Download Preset</button>
        </div>
        <div class="muted" id="modelMsg"></div>
      </section>
    </div>

    <section class="card" style="margin-top:12px;">
      <h3 style="margin:0 0 8px;">Transcribe File</h3>
      <div class="row">
        <input id="fileInput" type="file" accept="audio/*,video/*" />
      </div>
      <div class="row">
        <label>Language: <input id="langInput" value="auto" style="width:110px" /></label>
        <label>Model: <input id="runModelInput" value="ggml-base" style="width:160px" /></label>
        <button id="uploadBtn" class="primary">Start Transcription</button>
      </div>
      <div class="muted" id="uploadMsg"></div>
    </section>

    <section class="card" style="margin-top:12px;">
      <h3 style="margin:0 0 8px;">Jobs</h3>
      <table>
        <thead><tr><th>ID</th><th>Status</th><th>Progress</th><th>Model</th><th>File</th><th>Results</th><th>Actions</th></tr></thead>
        <tbody id="jobsBody"></tbody>
      </table>
    </section>
  </div>

<script>
const state = { jobs: [], models: [], presets: [], defaultModel: 'ggml-base' };

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
      '<td title=\"' + job.filePath + '\">' + job.filePath + '</td>' +
      '<td class=\"links\">' + jobResultLinks(job) + '</td>' +
      '<td>' + jobActions(job) + '</td>';
    body.appendChild(tr);
  }
}

async function refreshData() {
  try {
    await api('/healthz');
    document.getElementById('health').textContent = 'Daemon healthy';
    document.getElementById('health').className = 'status-ok';

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
    document.getElementById('health').textContent = 'Error: ' + err.message;
    document.getElementById('health').className = 'status-bad';
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
