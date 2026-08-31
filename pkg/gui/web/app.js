// Isthmus Studio Workbench - All 6 Advanced Engine Features
let currentDevice = 'local';
let currentPath = '.';
let allPeers = [];
let localNode = null;
let currentFileEntries = [];
let activeViewMode = 'grid'; // 'grid' or 'table'
let selectedFile = null;
let activeTransfers = [];
let transferPollInterval = null;
let editorFileContext = null;

window.addEventListener('DOMContentLoaded', () => {
  loadStatus();
  loadPeers();
  loadDirectory('local', '.');
  loadLogs();
  startTransferPolling();
  initSSEEvents();
});

// Activity Rail Navigation Switcher
function switchMainTab(tabId) {
  document.querySelectorAll('.tab-pane').forEach(el => el.classList.remove('active'));
  document.querySelectorAll('.rail-btn').forEach(el => el.classList.remove('active'));

  const pane = document.getElementById(`pane-${tabId}`);
  const railBtn = document.getElementById(`rail-${tabId}`);

  if (pane) pane.classList.add('active');
  if (railBtn) railBtn.classList.add('active');

  if (tabId === 'peers') loadPeers();
  if (tabId === 'clipboard') loadClipboard();
  if (tabId === 'diagnostics') populateDiagPeers();
  if (tabId === 'transfers') renderTransfers();
  if (tabId === 'logs') loadLogs();
  if (tabId === 'vault') loadVaultStatus();
  if (tabId === 'runner') populateRunnerFleet();
  if (tabId === 'mount') loadMountCommand();
  if (tabId === 'relay') loadRelayRoutes();
  if (tabId === 'terminal') {
    populateTerminalTargets(allPeers);
    setTimeout(() => {
      const inp = document.getElementById('terminal-input');
      if (inp) inp.focus();
    }, 100);
  }
}

// Toggle View Mode (Grid vs Table)
function setViewMode(mode) {
  activeViewMode = mode;
  document.getElementById('view-mode-grid').classList.toggle('active', mode === 'grid');
  document.getElementById('view-mode-table').classList.toggle('active', mode === 'table');

  document.getElementById('files-grid-container').style.display = mode === 'grid' ? 'grid' : 'none';
  document.getElementById('files-table-container').style.display = mode === 'table' ? 'block' : 'none';

  renderCurrentFiles();
}

// Load status from local daemon
async function loadStatus() {
  try {
    const res = await fetch('/api/status');
    if (!res.ok) return;
    const data = await res.json();
    localNode = data;

    const ipEl = document.getElementById('sidebar-virtual-ip');
    if (ipEl) ipEl.textContent = data.virtual_ip || '--';

    const sftpEl = document.getElementById('sidebar-sftp-port');
    if (sftpEl) sftpEl.textContent = data.sftp_port || '--';

    const tunEl = document.getElementById('sidebar-tunnel-port');
    if (tunEl) tunEl.textContent = data.listen_port || '--';

    const coordEl = document.getElementById('sidebar-coord-status');
    if (coordEl) coordEl.textContent = data.coord_server ? 'WAN Relay' : 'Direct LAN';

    const devNameEl = document.getElementById('sec-device-name');
    if (devNameEl) devNameEl.value = data.device_name || '';

    const devIdEl = document.getElementById('sec-device-id');
    if (devIdEl) devIdEl.value = data.device_id || '';

    const pubKeyEl = document.getElementById('sec-public-key');
    if (pubKeyEl) pubKeyEl.value = data.public_key || '';

    const sharedEl = document.getElementById('sec-shared-dir');
    if (sharedEl) sharedEl.value = data.shared_dir || '';
  } catch (err) {
    appendLog(`[ERR] Status error: ${err.message}`);
  }
}

// Load peers
async function loadPeers() {
  try {
    const res = await fetch('/api/peers');
    if (!res.ok) return;
    const peers = await res.json();
    allPeers = peers || [];

    renderTreeNodes(allPeers);
    renderPeersTable(allPeers);
    populateACLSelect(allPeers);
    populateTerminalTargets(allPeers);
    populateDiagPeers();
  } catch (err) {
    appendLog(`[ERR] Peers error: ${err.message}`);
  }
}

// Render Left Tree Nodes
function renderTreeNodes(peers) {
  const container = document.getElementById('tree-node-list');
  if (!container) return;

  const localName = localNode ? localNode.device_name : 'This Machine';
  const localIP = localNode ? localNode.virtual_ip : '10.77.0.1';

  container.innerHTML = `
    <div class="node-card-item ${currentDevice === 'local' ? 'active' : ''}" onclick="selectDevice('local', '${localName}')">
      <div class="node-avatar">
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
      </div>
      <div class="node-title-box">
        <div class="node-name">${escapeHTML(localName)}</div>
        <div class="node-subtext">${escapeHTML(localIP)} (Local)</div>
      </div>
    </div>
  `;

  peers.forEach(peer => {
    const item = document.createElement('div');
    item.className = `node-card-item ${currentDevice === peer.device_id ? 'active' : ''}`;
    item.onclick = () => selectDevice(peer.device_id, peer.device_name);

    item.innerHTML = `
      <div class="node-avatar">
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>
      </div>
      <div class="node-title-box">
        <div class="node-name">${escapeHTML(peer.device_name)}</div>
        <div class="node-subtext">${escapeHTML(peer.virtual_ip || peer.device_id.substring(0, 10))}</div>
      </div>
    `;
    container.appendChild(item);
  });
}

// Select active node
function selectDevice(deviceId, deviceName) {
  currentDevice = deviceId;
  currentPath = '.';

  const displayName = deviceName || (deviceId === 'local' ? 'This Machine' : deviceId);
  const statusName = document.getElementById('status-peer-name');
  if (statusName) statusName.textContent = displayName;

  closeInspector();
  updateBreadcrumbs('.');
  renderTreeNodes(allPeers);
  loadDirectory(deviceId, currentPath);
  appendLog(`[NAV] Switched to '${displayName}'`);
}

// Load Directory
async function loadDirectory(deviceId, path) {
  const gridContainer = document.getElementById('files-grid-container');
  const tableBody = document.getElementById('files-table-body');
  const emptyEl = document.getElementById('explorer-empty');

  if (emptyEl) emptyEl.style.display = 'none';
  if (gridContainer) gridContainer.innerHTML = `<div style="grid-column: 1/-1; padding: 40px; text-align: center; color: var(--text-muted);">Loading files...</div>`;
  if (tableBody) tableBody.innerHTML = `<tr><td colspan="4" style="text-align: center; padding: 40px; color: var(--text-muted);">Loading files...</td></tr>`;

  try {
    const res = await fetch(`/api/browse?peer=${encodeURIComponent(deviceId)}&path=${encodeURIComponent(path)}`);
    if (!res.ok) {
      const errData = await res.json();
      if (gridContainer) gridContainer.innerHTML = `<div style="grid-column: 1/-1; padding: 40px; text-align: center; color: var(--status-danger);">[ERROR] ${escapeHTML(errData.error || 'Access Denied')}</div>`;
      return;
    }

    const data = await res.json();
    currentFileEntries = data.entries || [];
    renderCurrentFiles();

    const countEl = document.getElementById('status-items-count');
    if (countEl) countEl.textContent = `${currentFileEntries.length} items`;

    const tierEl = document.getElementById('status-transport-tier');
    if (tierEl) tierEl.textContent = data.tier || 'Direct Mesh';
  } catch (err) {
    if (gridContainer) gridContainer.innerHTML = `<div style="grid-column: 1/-1; padding: 40px; text-align: center; color: var(--status-danger);">[ERROR] ${escapeHTML(err.message)}</div>`;
  }
}

function renderCurrentFiles() {
  const emptyEl = document.getElementById('explorer-empty');

  if (!currentFileEntries || currentFileEntries.length === 0) {
    if (emptyEl) emptyEl.style.display = 'block';
    document.getElementById('files-grid-container').innerHTML = '';
    document.getElementById('files-table-body').innerHTML = '';
    return;
  }

  if (emptyEl) emptyEl.style.display = 'none';

  if (activeViewMode === 'grid') {
    renderFileGrid(currentFileEntries);
  } else {
    renderFileTable(currentFileEntries);
  }
}

// Render Card Grid Mode
function renderFileGrid(entries) {
  const container = document.getElementById('files-grid-container');
  if (!container) return;
  container.innerHTML = '';

  if (currentPath !== '.' && currentPath !== '' && currentPath !== '/') {
    const parentCard = document.createElement('div');
    parentCard.className = 'file-card';
    parentCard.ondblclick = () => navigateUp();
    parentCard.innerHTML = `
      <div class="file-card-top">
        <span class="file-type-chip" style="color: var(--text-muted);">DIR</span>
        <button class="btn btn-sm" onclick="navigateUp()">Up</button>
      </div>
      <div class="file-card-name">.. (Parent Folder)</div>
      <div class="file-card-meta">
        <span>Directory</span>
        <span>--</span>
      </div>
    `;
    container.appendChild(parentCard);
  }

  entries.forEach(entry => {
    const card = document.createElement('div');
    card.className = `file-card ${selectedFile && selectedFile.name === entry.name ? 'selected' : ''}`;
    const isDir = entry.is_dir;
    const ext = isDir ? 'DIR' : getFileExtension(entry.name);
    const sizeStr = isDir ? '--' : formatBytes(entry.size);
    const dateStr = entry.modified ? new Date(entry.modified).toLocaleDateString() : '--';

    card.onclick = () => openInspector(entry);
    if (isDir) {
      card.ondblclick = () => enterDirectory(entry.name);
    }

    card.innerHTML = `
      <div class="file-card-top">
        <span class="file-type-chip">${escapeHTML(ext)}</span>
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="color: var(--text-muted);"><circle cx="12" cy="12" r="1"/><circle cx="12" cy="5" r="1"/><circle cx="12" cy="19" r="1"/></svg>
      </div>
      <div class="file-card-name">${escapeHTML(entry.name)}</div>
      <div class="file-card-meta">
        <span>${sizeStr}</span>
        <span>${dateStr}</span>
      </div>
    `;
    container.appendChild(card);
  });
}

// Render List Table Mode
function renderFileTable(entries) {
  const tbody = document.getElementById('files-table-body');
  if (!tbody) return;
  tbody.innerHTML = '';

  if (currentPath !== '.' && currentPath !== '' && currentPath !== '/') {
    const row = document.createElement('tr');
    row.ondblclick = () => navigateUp();
    row.innerHTML = `
      <td><strong>.. (Parent Folder)</strong></td>
      <td>--</td>
      <td>--</td>
      <td style="text-align: right;"><button class="btn btn-sm" onclick="navigateUp()">Up</button></td>
    `;
    tbody.appendChild(row);
  }

  entries.forEach(entry => {
    const row = document.createElement('tr');
    const isDir = entry.is_dir;
    const sizeStr = isDir ? '<DIR>' : formatBytes(entry.size);
    const dateStr = entry.modified ? new Date(entry.modified).toLocaleDateString() : '--';

    row.onclick = () => openInspector(entry);
    if (isDir) {
      row.ondblclick = () => enterDirectory(entry.name);
    }

    row.innerHTML = `
      <td><strong>${escapeHTML(entry.name)}</strong></td>
      <td style="font-family: var(--font-mono); font-size: 11.5px; color: var(--text-secondary);">${sizeStr}</td>
      <td style="font-family: var(--font-mono); font-size: 11.5px; color: var(--text-secondary);">${dateStr}</td>
      <td style="text-align: right;">
        ${!isDir ? `<button class="btn btn-sm btn-primary" onclick="downloadFile('${escapeHTML(entry.name)}')">Download</button>` : `<button class="btn btn-sm" onclick="enterDirectory('${escapeHTML(entry.name)}')">Open</button>`}
      </td>
    `;
    tbody.appendChild(row);
  });
}

// Open / Close Right File Inspector
function openInspector(entry) {
  selectedFile = entry;
  const inspector = document.getElementById('file-inspector');
  if (!inspector) return;

  inspector.style.display = 'flex';
  document.getElementById('insp-filename').textContent = entry.name;
  document.getElementById('insp-size').textContent = entry.is_dir ? 'Directory' : formatBytes(entry.size);
  document.getElementById('insp-date').textContent = entry.modified ? new Date(entry.modified).toLocaleString() : '--';
  document.getElementById('insp-node').textContent = currentDevice;

  const btnEdit = document.getElementById('insp-btn-edit');
  const btnStream = document.getElementById('insp-btn-stream');
  const btnShare = document.getElementById('insp-btn-share');
  const btnDown = document.getElementById('insp-btn-download');
  const btnDel = document.getElementById('insp-btn-delete');

  if (entry.is_dir) {
    btnEdit.style.display = 'none';
    btnStream.style.display = 'none';
    btnShare.style.display = 'none';
    btnDown.textContent = 'Open Folder';
    btnDown.onclick = () => enterDirectory(entry.name);
  } else {
    btnEdit.style.display = 'inline-flex';
    btnStream.style.display = 'inline-flex';
    btnShare.style.display = 'inline-flex';
    btnDown.textContent = 'Download';
    btnDown.onclick = () => downloadFile(entry.name);
    btnEdit.onclick = () => openCodeEditor(entry);
    btnStream.onclick = () => openMediaViewer(entry);
    btnShare.onclick = () => openShareModal(entry);
  }

  btnDel.onclick = () => deleteItem(entry.name);
  loadRevisions(entry);
}

function closeInspector() {
  selectedFile = null;
  const inspector = document.getElementById('file-inspector');
  if (inspector) inspector.style.display = 'none';
}

// 1. In-Browser Remote Code & Text Editor
async function openCodeEditor(entry) {
  editorFileContext = {
    peer: currentDevice,
    path: currentPath === '.' ? entry.name : `${currentPath}/${entry.name}`,
    name: entry.name,
  };

  const modal = document.getElementById('modal-code-editor');
  const titleEl = document.getElementById('editor-filename');
  const statusEl = document.getElementById('editor-status-pill');
  const textarea = document.getElementById('editor-textarea');

  titleEl.textContent = `${editorFileContext.peer}:${editorFileContext.path}`;
  statusEl.textContent = 'Loading...';
  statusEl.style.color = 'var(--text-muted)';
  textarea.value = '';
  modal.classList.add('active');

  try {
    const res = await fetch(`/api/file/content?peer=${encodeURIComponent(editorFileContext.peer)}&path=${encodeURIComponent(editorFileContext.path)}`);
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    textarea.value = data.content || '';
    statusEl.textContent = 'Saved';
    statusEl.style.color = 'var(--status-online)';
    textarea.focus();
  } catch (err) {
    statusEl.textContent = 'Read Error';
    statusEl.style.color = 'var(--status-danger)';
    textarea.value = `// Failed to read file: ${err.message}`;
  }
}

function onEditorInput() {
  const statusEl = document.getElementById('editor-status-pill');
  if (statusEl) {
    statusEl.textContent = 'Unsaved changes';
    statusEl.style.color = 'var(--status-warning)';
  }
}

function handleEditorShortcuts(event) {
  if ((event.ctrlKey || event.metaKey) && event.key === 's') {
    event.preventDefault();
    saveEditorContent();
  }
}

async function saveEditorContent() {
  if (!editorFileContext) return;
  const statusEl = document.getElementById('editor-status-pill');
  const textarea = document.getElementById('editor-textarea');
  statusEl.textContent = 'Saving...';

  try {
    const res = await fetch('/api/file/save', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        peer: editorFileContext.peer,
        path: editorFileContext.path,
        content: textarea.value,
      }),
    });

    if (!res.ok) throw new Error(await res.text());
    statusEl.textContent = 'Saved';
    statusEl.style.color = 'var(--status-online)';
    appendLog(`[EDITOR OK] Saved '${editorFileContext.path}' remotely.`);
    loadDirectory(currentDevice, currentPath);
    if (selectedFile) loadRevisions(selectedFile);
  } catch (err) {
    statusEl.textContent = 'Save Failed';
    statusEl.style.color = 'var(--status-danger)';
    alert(`Save failed: ${err.message}`);
  }
}

// 2. Direct In-Browser Media Streaming
function openMediaViewer(entry) {
  const filePath = currentPath === '.' ? entry.name : `${currentPath}/${entry.name}`;
  const streamURL = `/api/stream?peer=${encodeURIComponent(currentDevice)}&path=${encodeURIComponent(filePath)}`;
  const ext = getFileExtension(entry.name).toLowerCase();

  const modal = document.getElementById('modal-media-player');
  const title = document.getElementById('media-player-title');
  const video = document.getElementById('media-video');
  const audio = document.getElementById('media-audio');
  const img = document.getElementById('media-img');

  title.textContent = `Streaming: ${entry.name}`;
  video.style.display = 'none';
  audio.style.display = 'none';
  img.style.display = 'none';
  video.pause();
  audio.pause();

  if (['mp4', 'mkv', 'webm', 'mov'].includes(ext)) {
    video.src = streamURL;
    video.style.display = 'block';
    video.play();
  } else if (['mp3', 'wav', 'flac', 'ogg', 'm4a'].includes(ext)) {
    audio.src = streamURL;
    audio.style.display = 'block';
    audio.play();
  } else if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg'].includes(ext)) {
    img.src = streamURL;
    img.style.display = 'block';
  } else {
    alert(`Format '.${ext}' is not directly streamable. Downloading file instead.`);
    downloadFile(entry.name);
    return;
  }

  modal.classList.add('active');
}

function closeMediaModal() {
  const modal = document.getElementById('modal-media-player');
  const video = document.getElementById('media-video');
  const audio = document.getElementById('media-audio');
  if (video) video.pause();
  if (audio) audio.pause();
  if (modal) modal.classList.remove('active');
}

// 3. Magic Clipboard Sync
async function loadClipboard() {
  const list = document.getElementById('clipboard-history-list');
  if (!list) return;

  try {
    const res = await fetch('/api/clipboard');
    if (!res.ok) return;
    const data = await res.json();
    const history = data.history || [];

    if (history.length === 0) {
      list.innerHTML = `<div style="color: var(--text-muted); padding: 20px; text-align: center;">No items in mesh clipboard yet. Paste text above to broadcast.</div>`;
      return;
    }

    list.innerHTML = '';
    history.forEach(item => {
      const card = document.createElement('div');
      card.style.cssText = 'background: var(--bg-card); border: 1px solid var(--border-default); border-radius: var(--radius-sm); padding: 12px;';
      card.innerHTML = `
        <div style="display: flex; justify-content: space-between; margin-bottom: 6px; font-size: 11px;">
          <span style="color: var(--accent-primary); font-weight: 700;">${escapeHTML(item.source)}</span>
          <span style="color: var(--text-muted); font-family: var(--font-mono);">${new Date(item.timestamp).toLocaleTimeString()}</span>
        </div>
        <div style="font-family: var(--font-mono); font-size: 12px; color: var(--text-primary); white-space: pre-wrap; word-break: break-all; margin-bottom: 8px;">${escapeHTML(item.content)}</div>
        <div style="display: flex; justify-content: flex-end;">
          <button class="btn btn-sm" onclick="navigator.clipboard.writeText('${escapeHTML(item.content)}'); alert('Copied to local clipboard!');">Copy</button>
        </div>
      `;
      list.appendChild(card);
    });
  } catch (_) {}
}

async function pushClipboard() {
  const inp = document.getElementById('clipboard-push-input');
  const text = inp ? inp.value.trim() : '';
  if (!text) return;

  try {
    const res = await fetch('/api/clipboard', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content: text, source: localNode ? localNode.device_name : 'This Machine' }),
    });

    if (res.ok) {
      inp.value = '';
      loadClipboard();
      appendLog(`[CLIP OK] Synced ${text.length} chars to mesh clipboard.`);
    }
  } catch (err) {
    alert(`Clipboard error: ${err.message}`);
  }
}

// 4. Mesh Diagnostics (Ping & Speedtest)
function populateDiagPeers() {
  const sel = document.getElementById('diag-peer-select');
  if (!sel) return;
  sel.innerHTML = '';
  allPeers.forEach(p => {
    const opt = document.createElement('option');
    opt.value = p.device_id;
    opt.textContent = `${p.device_name} (${p.virtual_ip || p.device_id.slice(0, 10)})`;
    sel.appendChild(opt);
  });
}

async function runPingTest() {
  const sel = document.getElementById('diag-peer-select');
  const out = document.getElementById('diag-results-box');
  if (!sel || !sel.value) {
    alert('Please select a peer node.');
    return;
  }

  out.innerHTML = `[PING] Probing peer ${sel.options[sel.selectedIndex].text}...\n`;
  try {
    const res = await fetch('/api/diagnostics/ping', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ peer_id: sel.value }),
    });
    const data = await res.json();
    if (data.reachable) {
      out.innerHTML += `[SUCCESS] Ping roundtrip: ${data.latency_ms.toFixed(2)} ms | Target: ${data.endpoint}\n`;
    } else {
      out.innerHTML += `[FAILED] Peer unreachable: ${data.error || 'Connection timed out'}\n`;
    }
  } catch (err) {
    out.innerHTML += `[ERR] ${err.message}\n`;
  }
}

async function runSpeedtest() {
  const sel = document.getElementById('diag-peer-select');
  const out = document.getElementById('diag-results-box');
  if (!sel || !sel.value) {
    alert('Please select a peer node.');
    return;
  }

  out.innerHTML = `[SPEEDTEST] Slicing and streaming benchmark buffers to ${sel.options[sel.selectedIndex].text}...\n`;
  try {
    const res = await fetch('/api/diagnostics/speedtest', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ peer_id: sel.value, size_bytes: 4 * 1024 * 1024 }),
    });
    const data = await res.json();
    if (data.speed_mbps) {
      out.innerHTML += `[SUCCESS] Speedtest Result: ${data.speed_mbps.toFixed(2)} MB/s\nDuration: ${data.duration_seconds.toFixed(3)}s | Tested: ${formatBytes(data.bytes_tested)}\n`;
    } else {
      out.innerHTML += `[FAILED] ${data.error}\n`;
    }
  } catch (err) {
    out.innerHTML += `[ERR] ${err.message}\n`;
  }
}

// 5. Expiring Guest Share Links
function openShareModal(entry) {
  const modal = document.getElementById('modal-share-link');
  const fn = document.getElementById('share-modal-filename');
  const result = document.getElementById('share-link-result');
  const btn = document.getElementById('btn-generate-share');

  fn.value = entry.name;
  result.style.display = 'none';
  btn.disabled = false;
  btn.textContent = 'Generate Link';
  modal.classList.add('active');
}

async function generateShareLink() {
  if (!selectedFile) return;
  const filePath = currentPath === '.' ? selectedFile.name : `${currentPath}/${selectedFile.name}`;
  const ttl = parseInt(document.getElementById('share-modal-ttl').value, 10);
  const limit = parseInt(document.getElementById('share-modal-limit').value, 10);

  const btn = document.getElementById('btn-generate-share');
  btn.disabled = true;
  btn.textContent = 'Generating...';

  try {
    const res = await fetch('/api/share/create', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        peer_id: currentDevice,
        file_path: filePath,
        filename: selectedFile.name,
        ttl_minutes: ttl,
        max_downloads: limit,
      }),
    });

    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();

    const resultBox = document.getElementById('share-link-result');
    const urlInput = document.getElementById('share-result-url');
    resultBox.style.display = 'block';
    urlInput.value = data.share_url;
    urlInput.select();
    appendLog(`[SHARE OK] Generated expiring guest link: ${data.token}`);
  } catch (err) {
    alert(`Failed to generate share link: ${err.message}`);
  } finally {
    btn.disabled = false;
    btn.textContent = 'Generate Link';
  }
}

// 6. Time-Machine Revision Snapshots
async function loadRevisions(entry) {
  const container = document.getElementById('insp-revisions-list');
  if (!container) return;

  const filePath = currentPath === '.' ? entry.name : `${currentPath}/${entry.name}`;
  try {
    const res = await fetch(`/api/history?path=${encodeURIComponent(filePath)}`);
    if (!res.ok) return;
    const snaps = await res.json();

    if (!snaps || snaps.length === 0) {
      container.innerHTML = `<span style="color: var(--text-dim);">No prior snapshots</span>`;
      return;
    }

    container.innerHTML = '';
    snaps.forEach(s => {
      const row = document.createElement('div');
      row.style.cssText = 'display: flex; justify-content: space-between; align-items: center; background: var(--bg-card); padding: 4px 6px; border-radius: var(--radius-xs); border: 1px solid var(--border-subtle);';
      row.innerHTML = `
        <div style="display: flex; flex-direction: column;">
          <span style="color: var(--text-primary);">${new Date(s.timestamp).toLocaleTimeString()}</span>
          <span style="color: var(--text-muted); font-size: 9.5px;">${formatBytes(s.size)}</span>
        </div>
        <button class="btn btn-sm" style="font-size: 10px; padding: 2px 6px;" onclick="restoreSnapshot('${escapeHTML(s.id)}')">Rollback</button>
      `;
      container.appendChild(row);
    });
  } catch (_) {}
}

async function restoreSnapshot(snapshotId) {
  if (!selectedFile) return;
  const filePath = currentPath === '.' ? selectedFile.name : `${currentPath}/${selectedFile.name}`;
  if (!confirm(`Are you sure you want to rollback '${selectedFile.name}' to this version?`)) return;

  try {
    const res = await fetch('/api/history/restore', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: filePath, snapshot_id: snapshotId }),
    });

    if (res.ok) {
      appendLog(`[RESTORE OK] Reverted '${filePath}' to snapshot.`);
      alert('Snapshot restored.');
      loadDirectory(currentDevice, currentPath);
      loadRevisions(selectedFile);
    }
  } catch (err) {
    alert(`Rollback error: ${err.message}`);
  }
}

// Search Filter
function filterFiles(query) {
  query = query.toLowerCase().trim();
  if (!query) {
    renderCurrentFiles();
    return;
  }

  const filtered = currentFileEntries.filter(e => e.name.toLowerCase().includes(query));
  if (activeViewMode === 'grid') {
    renderFileGrid(filtered);
  } else {
    renderFileTable(filtered);
  }
}

// File Extension Helper
function getFileExtension(filename) {
  const idx = filename.lastIndexOf('.');
  if (idx === -1 || idx === 0) return 'FILE';
  return filename.substring(idx + 1).toUpperCase().slice(0, 4);
}

// Navigation Helpers
function enterDirectory(dirName) {
  currentPath = currentPath === '.' ? dirName : `${currentPath}/${dirName}`;
  updateBreadcrumbs(currentPath);
  closeInspector();
  loadDirectory(currentDevice, currentPath);
}

function navigateUp() {
  if (currentPath === '.' || currentPath === '' || currentPath === '/') return;
  const parts = currentPath.split('/');
  parts.pop();
  currentPath = parts.join('/') || '.';
  updateBreadcrumbs(currentPath);
  closeInspector();
  loadDirectory(currentDevice, currentPath);
}

function navigateTo(path) {
  currentPath = path || '.';
  updateBreadcrumbs(currentPath);
  closeInspector();
  loadDirectory(currentDevice, currentPath);
}

function updateBreadcrumbs(path) {
  const container = document.getElementById('stage-breadcrumbs');
  if (!container) return;

  container.innerHTML = `<span class="crumb-link" onclick="navigateTo('.')">Root</span>`;
  if (path === '.' || !path) return;

  const parts = path.split('/');
  let accumulated = '';
  parts.forEach((part, i) => {
    if (!part) return;
    accumulated = accumulated ? `${accumulated}/${part}` : part;
    const isLast = i === parts.length - 1;
    container.innerHTML += `
      <span class="crumb-slash">/</span>
      <span class="crumb-link ${isLast ? 'active-folder' : ''}" onclick="navigateTo('${accumulated}')">${escapeHTML(part)}</span>
    `;
  });
}

// File Actions: Download, Delete, Upload
async function downloadFile(filename) {
  const filePath = currentPath === '.' ? filename : `${currentPath}/${filename}`;
  appendLog(`[TRANS] Download queued: '${filePath}'`);
  window.open(`/api/download?peer=${encodeURIComponent(currentDevice)}&path=${encodeURIComponent(filePath)}`, '_blank');
}

async function deleteItem(name) {
  if (!confirm(`Are you sure you want to delete '${name}'?`)) return;
  const itemPath = currentPath === '.' ? name : `${currentPath}/${name}`;

  try {
    const res = await fetch('/api/delete', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ peer: currentDevice, path: itemPath }),
    });

    if (res.ok) {
      appendLog(`[DEL OK] Deleted '${itemPath}'`);
      closeInspector();
      loadDirectory(currentDevice, currentPath);
    } else {
      const err = await res.text();
      alert(`Delete failed: ${err}`);
    }
  } catch (err) {
    alert(`Delete error: ${err.message}`);
  }
}

async function handleFileUpload(event) {
  const files = event.target.files;
  if (!files || files.length === 0) return;

  for (let i = 0; i < files.length; i++) {
    const file = files[i];
    const formData = new FormData();
    formData.append('file', file);
    formData.append('peer', currentDevice);
    formData.append('path', currentPath);

    appendLog(`[UPLOAD] Uploading '${file.name}' (${formatBytes(file.size)})...`);

    try {
      const res = await fetch('/api/upload', { method: 'POST', body: formData });
      if (res.ok) {
        appendLog(`[UPLOAD OK] '${file.name}' transferred successfully.`);
      } else {
        const err = await res.text();
        appendLog(`[UPLOAD ERR] ${err}`);
      }
    } catch (err) {
      appendLog(`[UPLOAD ERR] Network error on '${file.name}': ${err.message}`);
    }
  }

  event.target.value = '';
  loadDirectory(currentDevice, currentPath);
}

// Modals
function showNewFolderModal() {
  const modal = document.getElementById('modal-new-folder');
  if (modal) modal.classList.add('active');
  const inp = document.getElementById('modal-folder-name');
  if (inp) {
    inp.value = '';
    inp.focus();
  }
}

async function submitNewFolder() {
  const inp = document.getElementById('modal-folder-name');
  const name = inp ? inp.value.trim() : '';
  if (!name) return;

  const targetPath = currentPath === '.' ? name : `${currentPath}/${name}`;
  try {
    const res = await fetch('/api/mkdir', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ peer: currentDevice, path: targetPath }),
    });

    if (res.ok) {
      appendLog(`[MKDIR OK] Created folder '${targetPath}'`);
      closeModals();
      loadDirectory(currentDevice, currentPath);
    } else {
      const err = await res.text();
      alert(`Folder creation failed: ${err}`);
    }
  } catch (err) {
    alert(`Error: ${err.message}`);
  }
}

function closeModals() {
  document.querySelectorAll('.modal-overlay').forEach(m => m.classList.remove('active'));
}

// Pairing Modal Logic
let activePairTab = 'pin';

function showAddPeerModal() {
  const modal = document.getElementById('modal-add-peer');
  if (modal) modal.classList.add('active');
  switchPairModalTab('pin');
}

function switchPairModalTab(tab) {
  activePairTab = tab;
  ['pin', 'join', 'manual'].forEach(t => {
    const tabBtn = document.getElementById(`pair-tab-${t}`);
    const panel = document.getElementById(`pair-panel-${t}`);
    if (tabBtn) tabBtn.classList.toggle('active', t === tab);
    if (panel) panel.style.display = t === tab ? 'block' : 'none';
  });

  const submitBtn = document.getElementById('btn-submit-pair');
  if (tab === 'pin') {
    if (submitBtn) submitBtn.style.display = 'none';
    generateMagicPairing();
  } else if (tab === 'join') {
    if (submitBtn) {
      submitBtn.style.display = 'inline-block';
      submitBtn.textContent = 'Connect & Pair';
    }
  } else {
    if (submitBtn) {
      submitBtn.style.display = 'inline-block';
      submitBtn.textContent = 'Authorize Peer';
    }
  }
}

async function generateMagicPairing() {
  const pinEl = document.getElementById('magic-pin-display');
  const qrImg = document.getElementById('magic-qr-img');
  const timerEl = document.getElementById('magic-pin-timer');

  if (pinEl) pinEl.textContent = 'Generating...';
  if (qrImg) qrImg.style.display = 'none';
  if (timerEl) timerEl.textContent = 'Contacting pairing engine...';

  try {
    const res = await fetch('/api/pairing/generate', { method: 'POST' });
    if (!res.ok) throw new Error('Failed to generate session');
    const session = await res.json();

    if (pinEl) pinEl.textContent = `${session.pin.slice(0, 3)} - ${session.pin.slice(3)}`;
    if (session.qr_base64_png && qrImg) {
      qrImg.src = session.qr_base64_png;
      qrImg.style.display = 'block';
    }
    if (timerEl) timerEl.textContent = 'Active for 5 minutes. Enter this PIN or scan QR.';
    appendLog(`[PAIR] Generated 1-Click PIN: ${session.pin}`);
  } catch (err) {
    if (pinEl) pinEl.textContent = 'Error';
    if (timerEl) timerEl.textContent = err.message;
  }
}

async function submitPairingAction() {
  if (activePairTab === 'join') {
    const target = document.getElementById('join-target-input').value.trim();
    if (!target) {
      alert('Please enter a 6-digit PIN or QR URL.');
      return;
    }
    const btn = document.getElementById('btn-submit-pair');
    btn.disabled = true;
    btn.textContent = 'Connecting...';

    try {
      const res = await fetch('/api/pairing/join', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ target }),
      });
      if (!res.ok) {
        const errText = await res.text();
        throw new Error(errText || 'Pairing failed');
      }
      const peer = await res.json();
      appendLog(`[PAIR OK] Paired with ${peer.device_name}`);
      closeModals();
      loadPeers();
    } catch (err) {
      alert(`Pairing error: ${err.message}`);
    } finally {
      btn.disabled = false;
      btn.textContent = 'Connect & Pair';
    }
  } else if (activePairTab === 'manual') {
    submitAddPeer();
  }
}

async function submitAddPeer() {
  const name = document.getElementById('modal-peer-name').value.trim();
  const id = document.getElementById('modal-peer-id').value.trim();
  const key = document.getElementById('modal-peer-key').value.trim();
  const ip = document.getElementById('modal-peer-ip').value.trim();

  if (!name || !id || !key) {
    alert('Please fill in all fields.');
    return;
  }

  try {
    const res = await fetch('/api/peers/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ device_name: name, device_id: id, public_key: key, virtual_ip: ip || '10.77.0.2' }),
    });

    if (res.ok) {
      appendLog(`[PEER OK] Added peer '${name}'`);
      closeModals();
      loadPeers();
    }
  } catch (err) {
    alert(`Error: ${err.message}`);
  }
}

// Render Peers Table
function renderPeersTable(peers) {
  const tbody = document.getElementById('peers-table-body');
  if (!tbody) return;
  tbody.innerHTML = '';

  if (peers.length === 0) {
    tbody.innerHTML = `<tr><td colspan="6" style="text-align: center; padding: 40px; color: var(--text-muted);">No peer nodes. Click 'Pair New Device' to add one.</td></tr>`;
    return;
  }

  peers.forEach(peer => {
    const row = document.createElement('tr');
    row.innerHTML = `
      <td><strong>${escapeHTML(peer.device_name)}</strong></td>
      <td style="font-family: var(--font-mono); font-size: 11.5px; color: var(--text-secondary);">${escapeHTML(peer.device_id)}</td>
      <td style="font-family: var(--font-mono); font-size: 11.5px; color: var(--accent-primary); font-weight: 700;">${escapeHTML(peer.virtual_ip || '--')}</td>
      <td><span class="file-type-chip">Tier ${peer.transport_tier || 1}</span></td>
      <td><span class="file-type-chip" style="color: var(--status-online);">Read / Write</span></td>
      <td style="text-align: right;">
        <button class="btn btn-sm btn-primary" onclick="selectDevice('${peer.device_id}', '${peer.device_name}'); switchMainTab('explorer');">Browse</button>
      </td>
    `;
    tbody.appendChild(row);
  });
}

// ACL Policy
function populateACLSelect(peers) {
  const select = document.getElementById('acl-peer-select');
  if (!select) return;
  select.innerHTML = '';

  peers.forEach(peer => {
    const opt = document.createElement('option');
    opt.value = peer.device_id;
    opt.textContent = `${peer.device_name} (${peer.virtual_ip || peer.device_id.substring(0, 10)})`;
    select.appendChild(opt);
  });

  loadPeerACLDetails();
}

function loadPeerACLDetails() {
  const select = document.getElementById('acl-peer-select');
  if (!select || !select.value) return;
  const peer = allPeers.find(p => p.device_id === select.value);
  if (!peer) return;

  const acl = peer.acl || { allow_read: true, allow_write: true, allowed_paths: [], blocked_paths: ['.ssh', '.env', '.git'] };
  document.getElementById('acl-allow-read').checked = acl.allow_read !== false;
  document.getElementById('acl-allow-write').checked = acl.allow_write !== false;
  document.getElementById('acl-allowed-paths').value = (acl.allowed_paths || []).join(', ');
  document.getElementById('acl-blocked-paths').value = (acl.blocked_paths || ['.ssh', '.env', '.git']).join(', ');
}

async function savePeerACL() {
  const select = document.getElementById('acl-peer-select');
  if (!select || !select.value) return;

  const allowRead = document.getElementById('acl-allow-read').checked;
  const allowWrite = document.getElementById('acl-allow-write').checked;
  const allowed = document.getElementById('acl-allowed-paths').value.split(',').map(s => s.trim()).filter(s => s.length > 0);
  const blocked = document.getElementById('acl-blocked-paths').value.split(',').map(s => s.trim()).filter(s => s.length > 0);

  try {
    const res = await fetch('/api/acl', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ peer_id: select.value, allow_read: allowRead, allow_write: allowWrite, allowed_paths: allowed, blocked_paths: blocked }),
    });

    if (res.ok) {
      appendLog(`[SEC OK] Saved ACL for '${select.value}'.`);
      alert('Security policy saved.');
      loadPeers();
    }
  } catch (err) {
    appendLog(`[ERR] Failed to save ACL: ${err.message}`);
  }
}

// Terminal Logic
let activeTerminalTarget = 'local';

function populateTerminalTargets(peers) {
  const sel = document.getElementById('terminal-target-select');
  if (!sel) return;
  const currentVal = sel.value;
  sel.innerHTML = '<option value="local">This Machine (Local Host)</option>';
  peers.forEach(p => {
    const opt = document.createElement('option');
    opt.value = p.device_id;
    opt.textContent = `${p.device_name} (${p.virtual_ip})`;
    sel.appendChild(opt);
  });
  if (currentVal) sel.value = currentVal;
}

function switchTerminalTarget() {
  const sel = document.getElementById('terminal-target-select');
  activeTerminalTarget = sel ? sel.value : 'local';
  const promptEl = document.getElementById('terminal-prompt');
  if (promptEl && sel && sel.selectedIndex >= 0) {
    promptEl.textContent = `${sel.options[sel.selectedIndex].text.split(' ')[0].toLowerCase()}$ `;
  }
}

function connectTerminal() {
  const sel = document.getElementById('terminal-target-select');
  const targetName = sel ? sel.options[sel.selectedIndex].text : 'Local';
  const out = document.getElementById('terminal-output');
  out.innerHTML += `\n[SYSTEM] Connected session to ${escapeHTML(targetName)}.\nType terminal commands below.\n`;
  switchTerminalTarget();
  const inp = document.getElementById('terminal-input');
  if (inp) inp.focus();
}

function clearTerminal() {
  const out = document.getElementById('terminal-output');
  if (out) out.innerHTML = 'Isthmus Remote Terminal Ready.\n';
}

function handleTerminalKey(event) {
  if (event.key === 'Enter') {
    const input = document.getElementById('terminal-input');
    const cmd = input.value.trim();
    if (!cmd) return;

    const out = document.getElementById('terminal-output');
    const prompt = document.getElementById('terminal-prompt').textContent;
    out.innerHTML += `\n${escapeHTML(prompt)} ${escapeHTML(cmd)}\n`;
    input.value = '';

    if (cmd === 'clear') {
      clearTerminal();
      return;
    }
    if (cmd === 'help') {
      out.innerHTML += `Available commands: status, devices, dir, ls, ipconfig, whoami, clear\n`;
      return;
    }

    if (cmd === 'status') {
      out.innerHTML += `Device: ${localNode ? localNode.device_name : 'node-local'} | IP: ${localNode ? localNode.virtual_ip : '10.77.0.1'} | Active Peers: ${allPeers.length}\n`;
      return;
    }
    if (cmd === 'devices' || cmd === 'peers') {
      allPeers.forEach(p => {
        out.innerHTML += ` - ${p.device_name} [${p.device_id.slice(0, 12)}...] (${p.virtual_ip})\n`;
      });
      return;
    }

    fetch('/api/terminal/exec', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ target: activeTerminalTarget, command: cmd }),
    })
      .then(res => res.json())
      .then(data => {
        if (data.output) out.innerHTML += escapeHTML(data.output) + '\n';
        const container = document.getElementById('terminal-container');
        if (container) container.scrollTop = container.scrollHeight;
      })
      .catch(err => {
        out.innerHTML += `[ERR] ${escapeHTML(err.message)}\n`;
      });
  }
}

// Transfer Polling
function startTransferPolling() {
  transferPollInterval = setInterval(fetchTransfers, 2000);
}

async function fetchTransfers() {
  try {
    const res = await fetch('/api/transfers');
    if (!res.ok) return;
    const transfers = await res.json();
    activeTransfers = transfers || [];

    const badge = document.getElementById('transfers-badge');
    const running = activeTransfers.filter(t => t.status === 'running').length;
    if (badge) {
      badge.style.display = running > 0 ? 'inline-block' : 'none';
      badge.textContent = running;
    }

    const panel = document.getElementById('pane-transfers');
    if (panel && panel.classList.contains('active')) {
      renderTransfers();
    }
  } catch (_) {}
}

function renderTransfers() {
  const container = document.getElementById('transfers-queue-list');
  if (!container) return;

  if (activeTransfers.length === 0) {
    container.innerHTML = `<div style="text-align: center; padding: 40px; color: var(--text-muted);">No active or recent transfers.</div>`;
    return;
  }

  container.innerHTML = '';
  activeTransfers.forEach(t => {
    const pct = t.total > 0 ? Math.round((t.transferred / t.total) * 100) : 0;
    const card = document.createElement('div');
    card.style.cssText = 'background: var(--bg-card); border: 1px solid var(--border-default); border-radius: var(--radius-md); padding: 14px; margin-bottom: 10px;';
    card.innerHTML = `
      <div style="display: flex; justify-content: space-between; margin-bottom: 8px;">
        <span style="font-weight: 700; color: #ffffff;">${escapeHTML(t.filename)}</span>
        <span style="font-family: var(--font-mono); font-size: 11px; color: var(--accent-primary); font-weight: 700;">${pct}% &bull; ${t.speed ? (t.speed / 1024 / 1024).toFixed(2) : '0.00'} MB/s</span>
      </div>
      <div style="height: 6px; background: var(--bg-canvas); border-radius: 3px; overflow: hidden; border: 1px solid var(--border-default);">
        <div style="width: ${pct}%; height: 100%; background: var(--accent-primary); transition: width 200ms ease;"></div>
      </div>
      <div style="display: flex; justify-content: space-between; margin-top: 6px; font-size: 11px; color: var(--text-muted); font-family: var(--font-mono);">
        <span>${formatBytes(t.transferred)} / ${formatBytes(t.total)}</span>
        <span>Peer: ${escapeHTML(t.peer_name || t.peer)}</span>
      </div>
    `;
    container.appendChild(card);
  });
}

function clearCompletedTransfers() {
  activeTransfers = activeTransfers.filter(t => t.status === 'running');
  renderTransfers();
}

// Logs Viewer
async function loadLogs() {
  try {
    const res = await fetch('/api/logs');
    if (!res.ok) return;
    const logs = await res.json();
    const consoleEl = document.getElementById('console-logs');
    if (consoleEl && Array.isArray(logs) && logs.length > 0) {
      consoleEl.innerHTML = logs.map(l => escapeHTML(l)).join('<br>') + '<br>';
      consoleEl.scrollTop = consoleEl.scrollHeight;
    }
  } catch (_) {}
}

function appendLog(msg) {
  const consoleEl = document.getElementById('console-logs');
  if (!consoleEl) return;
  const time = new Date().toLocaleTimeString();
  consoleEl.innerHTML += `[${time}] ${escapeHTML(msg)}<br>`;
  consoleEl.scrollTop = consoleEl.scrollHeight;
}

function refreshAll() {
  loadStatus();
  loadPeers();
  loadDirectory(currentDevice, currentPath);
}

function formatBytes(bytes) {
  if (bytes === 0 || !bytes) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

function escapeHTML(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

// ----------------------------------------------------
// 7. Real-Time SSE Push Event Receiver & Toast Engine
// ----------------------------------------------------
function initSSEEvents() {
  try {
    const evtSource = new EventSource('/api/events/stream');
    evtSource.onmessage = function(event) {
      if (!event.data) return;
      try {
        const ev = JSON.parse(event.data);
        if (ev.type === 'init') return;
        showToast(ev.title, ev.message, ev.level || 'info');
        appendLog(`[EVENT] ${ev.title}: ${ev.message}`);
      } catch (_) {}
    };
    evtSource.onerror = function() {
      // Reconnection handled automatically by browser
    };
  } catch (_) {}
}

function showToast(title, message, level = 'info') {
  const container = document.getElementById('toast-container');
  if (!container) return;

  const toast = document.createElement('div');
  toast.className = `toast toast-${level}`;
  toast.innerHTML = `
    <div class="toast-header">
      <span class="toast-title">${escapeHTML(title)}</span>
      <span class="toast-time">${new Date().toLocaleTimeString()}</span>
    </div>
    <div class="toast-body">${escapeHTML(message)}</div>
  `;

  container.appendChild(toast);

  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transform = 'translateX(40px)';
    setTimeout(() => toast.remove(), 250);
  }, 4500);
}

// ----------------------------------------------------
// 8. Zero-Trust Encrypted File Vault
// ----------------------------------------------------
async function loadVaultStatus() {
  try {
    const res = await fetch('/api/vault/status');
    if (!res.ok) return;
    const st = await res.json();

    const badge = document.getElementById('vault-status-badge');
    if (badge) {
      if (st.unlocked) {
        badge.textContent = 'UNLOCKED';
        badge.style.background = 'rgba(16, 185, 129, 0.2)';
        badge.style.color = 'var(--status-online)';
      } else {
        badge.textContent = 'LOCKED';
        badge.style.background = 'rgba(239, 68, 68, 0.2)';
        badge.style.color = 'var(--status-danger)';
      }
    }

    const countEl = document.getElementById('vault-files-count');
    if (countEl) countEl.textContent = `${st.encrypted_files_count} encrypted files (.enc)`;

    const dirEl = document.getElementById('vault-dir-path');
    if (dirEl && st.vault_directory) dirEl.textContent = st.vault_directory;
  } catch (_) {}
}

async function unlockVault() {
  const pass = document.getElementById('vault-passphrase').value;
  const dur = parseInt(document.getElementById('vault-duration').value, 10) || 30;
  if (!pass) {
    alert('Please enter a master passphrase.');
    return;
  }

  try {
    const res = await fetch('/api/vault/unlock', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ passphrase: pass, duration_minutes: dur }),
    });

    if (!res.ok) throw new Error(await res.text());
    document.getElementById('vault-passphrase').value = '';
    loadVaultStatus();
    showToast('Vault Unlocked', `Zero-Trust Vault unlocked for ${dur} minutes`, 'success');
  } catch (err) {
    alert(`Unlock failed: ${err.message}`);
  }
}

async function lockVault() {
  try {
    await fetch('/api/vault/lock', { method: 'POST' });
    loadVaultStatus();
    showToast('Vault Locked', 'Master keys securely wiped from memory', 'warning');
  } catch (_) {}
}

async function encryptFileToVault() {
  const fileInput = document.getElementById('vault-encrypt-file').value.trim();
  const pass = prompt('Enter encryption passphrase for this file:');
  if (!fileInput || !pass) return;

  try {
    const res = await fetch('/api/vault/encrypt', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: fileInput, passphrase: pass }),
    });

    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    document.getElementById('vault-encrypt-file').value = '';
    loadVaultStatus();
    showToast('File Encrypted', `Encrypted '${fileInput}' -> '${data.encrypted_path}'`, 'success');
  } catch (err) {
    alert(`Encryption error: ${err.message}`);
  }
}

async function decryptFileFromVault() {
  const fileInput = document.getElementById('vault-decrypt-file').value.trim();
  const pass = prompt('Enter master passphrase to decrypt this file:');
  if (!fileInput || !pass) return;

  try {
    const res = await fetch('/api/vault/decrypt', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enc_path: fileInput, passphrase: pass }),
    });

    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    document.getElementById('vault-decrypt-file').value = '';
    loadVaultStatus();
    showToast('File Decrypted', `Decrypted '${fileInput}' -> '${data.decrypted_path}'`, 'success');
  } catch (err) {
    alert(`Decryption error: ${err.message}`);
  }
}

// ----------------------------------------------------
// 9. Distributed Mesh Task & Script Runner
// ----------------------------------------------------
function populateRunnerFleet() {
  const sel = document.getElementById('runner-target-select');
  if (!sel) return;

  sel.innerHTML = `
    <option value="all">⚡ All Connected Mesh Nodes (Simultaneous Broadcast)</option>
    <option value="local">💻 This Local Machine</option>
  `;

  allPeers.forEach(p => {
    const opt = document.createElement('option');
    opt.value = p.device_id;
    opt.textContent = `📡 ${p.device_name} (${p.virtual_ip || p.device_id.slice(0, 10)})`;
    sel.appendChild(opt);
  });
}

function applyRunnerTemplate(name) {
  const templates = {
    'System Uptime': 'Get-CimInstance Win32_OperatingSystem | Select-Object LastBootUpTime',
    'Disk Space': 'Get-PSDrive -PSProvider FileSystem | Select-Object Name, Used, Free',
    'Active Processes': 'Get-Process | Sort-Object CPU -Descending | Select-Object -First 5',
    'Network Status': 'Get-NetIPAddress -AddressFamily IPv4 | Select-Object IPAddress, InterfaceAlias',
  };

  const cmdInput = document.getElementById('runner-cmd-input');
  if (cmdInput && templates[name]) {
    cmdInput.value = templates[name];
    cmdInput.focus();
  }
}

async function dispatchMeshJob() {
  const sel = document.getElementById('runner-target-select');
  const cmd = document.getElementById('runner-cmd-input').value.trim();
  const stream = document.getElementById('runner-results-stream');
  const btn = document.getElementById('btn-dispatch-runner');

  if (!cmd) {
    alert('Please enter a shell command or script.');
    return;
  }

  btn.disabled = true;
  btn.textContent = 'Executing...';
  stream.innerHTML = `<div style="background: #000000; border: 1px solid var(--border-default); border-radius: var(--radius-md); padding: 16px; font-family: var(--font-mono); font-size: 12px; color: var(--accent-primary);">[DISPATCH] Broadcasting job '${escapeHTML(cmd)}'...</div>`;

  try {
    const targets = sel.value === 'all' ? ['all'] : [sel.value];
    const res = await fetch('/api/runner/exec', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ command: cmd, targets: targets, timeout_seconds: 15 }),
    });

    if (!res.ok) throw new Error(await res.text());
    const batch = await res.json();

    stream.innerHTML = '';
    batch.results.forEach(r => {
      const card = document.createElement('div');
      card.style.cssText = 'background: var(--bg-card); border: 1px solid var(--border-default); border-radius: var(--radius-md); overflow: hidden;';
      card.innerHTML = `
        <div style="background: var(--bg-sidebar); border-bottom: 1px solid var(--border-default); padding: 8px 14px; display: flex; justify-content: space-between; align-items: center; font-size: 11.5px;">
          <span style="font-weight: 700; color: #ffffff;">${escapeHTML(r.target_name)} (${escapeHTML(r.target_id)})</span>
          <div style="display: flex; gap: 8px; align-items: center;">
            <span style="font-family: var(--font-mono); color: var(--text-muted);">${r.duration_ms.toFixed(2)} ms</span>
            <span style="padding: 1px 6px; border-radius: 3px; font-family: var(--font-mono); font-weight: 700; font-size: 10px; background: ${r.exit_code === 0 ? 'rgba(16, 185, 129, 0.2)' : 'rgba(239, 68, 68, 0.2)'}; color: ${r.exit_code === 0 ? 'var(--status-online)' : 'var(--status-danger)'};">EXIT ${r.exit_code}</span>
          </div>
        </div>
        <div style="padding: 14px; background: #000000; font-family: var(--font-mono); font-size: 12px; color: var(--text-primary); white-space: pre-wrap; word-break: break-all; max-height: 220px; overflow-y: auto; line-height: 1.5;">${escapeHTML(r.stdout || r.error || 'No output')}</div>
      `;
      stream.appendChild(card);
    });
  } catch (err) {
    stream.innerHTML = `<div style="color: var(--status-danger); padding: 14px; font-family: var(--font-mono);">[ERR] Execution failed: ${escapeHTML(err.message)}</div>`;
  } finally {
    btn.disabled = false;
    btn.textContent = '⚡ Dispatch to Fleet';
  }
}

// ----------------------------------------------------
// 10. Virtual Drive / WebDAV Mount Engine
// ----------------------------------------------------
async function loadMountCommand() {
  const drive = document.getElementById('mount-drive-select').value || 'Z:';
  try {
    const res = await fetch(`/api/webdav/mount?mount_point=${encodeURIComponent(drive)}`);
    if (!res.ok) return;
    const data = await res.json();
    const cmdInput = document.getElementById('mount-command-text');
    if (cmdInput) cmdInput.value = data.mount_command;
    const urlEl = document.getElementById('mount-webdav-url');
    if (urlEl) urlEl.textContent = data.webdav_url;
  } catch (_) {}
}

function copyMountCommand() {
  const cmd = document.getElementById('mount-command-text');
  if (!cmd || !cmd.value) return;
  navigator.clipboard.writeText(cmd.value);
  showToast('Command Copied', 'Paste into PowerShell to mount virtual drive Z:', 'success');
}

// ----------------------------------------------------
// 11. Multi-Hop P2P Mesh Relay Router
// ----------------------------------------------------
async function loadRelayRoutes() {
  const tbody = document.getElementById('relay-routes-body');
  if (!tbody) return;

  try {
    const res = await fetch('/api/relay/routes');
    if (!res.ok) return;
    const routes = await res.json();

    if (!routes || routes.length === 0) {
      tbody.innerHTML = `<tr><td colspan="6" style="text-align: center; color: var(--text-muted); padding: 30px;">No active multi-hop routing paths currently mapped. Connect peers to calculate hops.</td></tr>`;
      return;
    }

    tbody.innerHTML = '';
    routes.forEach(r => {
      const hopCount = r.hops ? r.hops.length : 1;
      const isDirect = hopCount <= 2;
      const hopsStr = r.hops ? r.hops.map(h => escapeHTML(h.device_name)).join(' &rarr; ') : escapeHTML(r.target_name);

      const row = document.createElement('tr');
      row.innerHTML = `
        <td style="font-weight: 700; color: #ffffff;">${escapeHTML(r.target_name)}</td>
        <td style="font-family: var(--font-mono); font-size: 11px; color: var(--text-secondary);">${escapeHTML(r.target_id)}</td>
        <td style="font-family: var(--font-mono); font-size: 11px; color: var(--accent-primary);">${hopsStr}</td>
        <td><span style="font-size: 10px; padding: 2px 6px; border-radius: 3px; background: ${isDirect ? 'var(--bg-elevated)' : 'rgba(245, 158, 11, 0.2)'}; color: ${isDirect ? 'var(--text-primary)' : 'var(--accent-primary)'}; font-weight: 700;">${isDirect ? 'Direct (1-Hop)' : `Multi-Hop (${hopCount} Hops)`}</span></td>
        <td style="font-family: var(--font-mono); font-size: 11px; color: var(--text-muted);">${(r.total_latency / 1000000).toFixed(1)} ms</td>
        <td style="text-align: right;"><span style="color: var(--status-online); font-weight: 700; font-size: 11px;">Active</span></td>
      `;
      tbody.appendChild(row);
    });
  } catch (_) {}
}

