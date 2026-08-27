// Isthmus Desktop GUI Application Logic
let currentDevice = 'local';
let currentPath = '.';
let allPeers = [];
let localNode = null;
let activeTransfers = [];
let transferPollInterval = null;

// Initialize app on load
window.addEventListener('DOMContentLoaded', () => {
  loadStatus();
  loadPeers();
  loadDirectory('local', '.');
  startTransferPolling();
});

// Tab navigation
function switchTab(tabId) {
  document.querySelectorAll('.tab-content').forEach(el => el.classList.remove('active'));
  document.querySelectorAll('.tab-btn').forEach(el => el.classList.remove('active'));
  
  const content = document.getElementById(`tab-${tabId}`);
  const btn = document.getElementById(`tab-btn-${tabId}`);
  
  if (content) content.classList.add('active');
  if (btn) btn.classList.add('active');

  if (tabId === 'peers') loadPeers();
  if (tabId === 'transfers') renderTransfers();
}

// Fetch local node status
async function loadStatus() {
  try {
    const res = await fetch('/api/status');
    if (!res.ok) return;
    const data = await res.json();
    localNode = data;

    document.getElementById('local-device-name').textContent = `${data.device_name} (Local)`;
    document.getElementById('sidebar-virtual-ip').textContent = data.virtual_ip || '10.77.0.1';
    document.getElementById('sidebar-sftp-port').textContent = data.sftp_port || '2222';
    document.getElementById('sidebar-tunnel-port').textContent = data.listen_port || '51820';
    document.getElementById('sidebar-coord-status').textContent = data.coord_server ? 'WAN Connected' : 'LAN Mode';

    // Populate Security Tab
    document.getElementById('sec-device-name').value = data.device_name || '';
    document.getElementById('sec-device-id').value = data.device_id || '';
    document.getElementById('sec-public-key').value = data.public_key || '';
    document.getElementById('sec-shared-dir').value = data.shared_dir || '';
  } catch (err) {
    appendLog(`[ERR] Failed to load status: ${err.message}`);
  }
}

// Fetch peers list
async function loadPeers() {
  try {
    const res = await fetch('/api/peers');
    if (!res.ok) return;
    const peers = await res.json();
    allPeers = peers;

    renderSidebarPeers(peers);
    renderPeersGrid(peers);
    populateACLSelect(peers);
  } catch (err) {
    appendLog(`[ERR] Failed to load peers: ${err.message}`);
  }
}

// Render left sidebar device list
function renderSidebarPeers(peers) {
  const container = document.getElementById('device-list');
  // Keep local device
  container.innerHTML = `
    <div class="device-item ${currentDevice === 'local' ? 'active' : ''}" onclick="selectDevice('local')">
      <span class="device-icon">[PC]</span>
      <div class="device-info">
        <div class="device-name">${localNode ? localNode.device_name : 'This Machine'} (Local)</div>
        <div class="device-meta">${localNode ? localNode.virtual_ip : '10.77.0.1'}</div>
      </div>
      <span class="device-badge badge-lan">LOCAL</span>
    </div>
  `;

  peers.forEach(peer => {
    const item = document.createElement('div');
    item.className = `device-item ${currentDevice === peer.device_id ? 'active' : ''}`;
    item.onclick = () => selectDevice(peer.device_id, peer.device_name);

    let badgeClass = 'badge-lan';
    let badgeText = 'LAN';
    if (peer.transport_tier === 2) {
      badgeClass = 'badge-wan';
      badgeText = 'WAN';
    } else if (peer.transport_tier === 3) {
      badgeClass = 'badge-relay';
      badgeText = 'RELAY';
    }

    item.innerHTML = `
      <span class="device-icon">[NODE]</span>
      <div class="device-info">
        <div class="device-name">${escapeHTML(peer.device_name)}</div>
        <div class="device-meta">${escapeHTML(peer.virtual_ip || peer.device_id.substring(0, 12))}</div>
      </div>
      <span class="device-badge ${badgeClass}">${badgeText}</span>
    `;
    container.appendChild(item);
  });
}

// Render Peers Directory Grid Tab
function renderPeersGrid(peers) {
  const grid = document.getElementById('peers-grid');
  if (!grid) return;
  
  if (peers.length === 0) {
    grid.innerHTML = `<div style="grid-column: 1/-1; padding: 24px; color: var(--text-muted); font-family: var(--font-mono);">No remote peer devices configured yet. Click '[+] Add New Peer' to pair a node.</div>`;
    return;
  }

  grid.innerHTML = '';
  peers.forEach(peer => {
    const card = document.createElement('div');
    card.className = 'retro-card';
    card.innerHTML = `
      <div class="card-title" style="display: flex; justify-content: space-between;">
        <span>[PC] ${escapeHTML(peer.device_name)}</span>
        <span style="font-size: 11px; color: var(--text-blue);">${escapeHTML(peer.virtual_ip || '10.77.0.x')}</span>
      </div>
      <div style="font-family: var(--font-mono); font-size: 11px; color: var(--text-muted); display: flex; flex-direction: column; gap: 4px; margin: 8px 0;">
        <div>ID: <span style="color: #fff;">${escapeHTML(peer.device_id)}</span></div>
        <div>Public Key: <span style="color: var(--text-cyan);">${escapeHTML(peer.public_key.substring(0, 24))}...</span></div>
        <div>Transport: <span style="color: #00ff66;">Tier ${peer.transport_tier || 1}</span></div>
      </div>
      <div style="display: flex; gap: 6px; margin-top: 10px;">
        <button class="retro-btn primary" onclick="selectDevice('${peer.device_id}', '${peer.device_name}'); switchTab('explorer');">Explore Files</button>
        <button class="retro-btn" onclick="removePeer('${peer.device_id}')">Remove</button>
      </div>
    `;
    grid.appendChild(card);
  });
}

// Select active device to browse
function selectDevice(deviceId, deviceName) {
  currentDevice = deviceId;
  currentPath = '.';
  document.getElementById('path-input').value = currentPath;

  const displayName = deviceName || (deviceId === 'local' ? 'Local Host' : deviceId);
  document.getElementById('status-peer-name').textContent = displayName;
  
  renderSidebarPeers(allPeers);
  loadDirectory(deviceId, currentPath);
  appendLog(`[NAV] Switched active device to '${displayName}'`);
}

// Load directory files
async function loadDirectory(deviceId, path) {
  const tbody = document.getElementById('file-table-body');
  tbody.innerHTML = `<tr><td colspan="5" style="text-align: center; padding: 24px; color: var(--text-muted);">Scanning directory '${escapeHTML(path)}'...</td></tr>`;

  try {
    const res = await fetch(`/api/browse?peer=${encodeURIComponent(deviceId)}&path=${encodeURIComponent(path)}`);
    if (!res.ok) {
      const errData = await res.json();
      tbody.innerHTML = `<tr><td colspan="5" style="text-align: center; padding: 24px; color: #ff5555;">[ERR] ${escapeHTML(errData.error || 'Failed to read directory')}</td></tr>`;
      return;
    }

    const data = await res.json();
    renderFileTable(data.entries || []);
    document.getElementById('status-items-count').textContent = `${data.entries ? data.entries.length : 0} items`;
    document.getElementById('status-transport-tier').textContent = data.tier || 'Direct Connection';
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="5" style="text-align: center; padding: 24px; color: #ff5555;">[ERR] Connection error: ${escapeHTML(err.message)}</td></tr>`;
  }
}

// Render file rows in table
function renderFileTable(entries) {
  const tbody = document.getElementById('file-table-body');
  tbody.innerHTML = '';

  if (entries.length === 0) {
    tbody.innerHTML = `<tr><td colspan="5" style="text-align: center; padding: 24px; color: var(--text-muted);">Folder is empty. Drag and drop files here to upload.</td></tr>`;
    return;
  }

  // Parent dir entry if not at root
  if (currentPath !== '.' && currentPath !== '' && currentPath !== '/') {
    const row = document.createElement('tr');
    row.className = 'file-row';
    row.ondblclick = () => navigateUp();
    row.innerHTML = `
      <td class="type-dir">[DIR]</td>
      <td><strong>.. (Parent Directory)</strong></td>
      <td>&lt;DIR&gt;</td>
      <td>--</td>
      <td><button class="retro-btn" onclick="navigateUp()">Up [^]</button></td>
    `;
    tbody.appendChild(row);
  }

  entries.forEach(entry => {
    const row = document.createElement('tr');
    row.className = 'file-row';

    const isDir = entry.is_dir;
    const typeLabel = isDir ? '[DIR]' : '[FILE]';
    const typeClass = isDir ? 'type-dir' : 'type-file';
    const sizeStr = isDir ? '<DIR>' : formatBytes(entry.size);
    const dateStr = entry.modified ? new Date(entry.modified).toLocaleString() : '--';

    row.onclick = () => {
      document.querySelectorAll('.file-row').forEach(r => r.classList.remove('selected'));
      row.classList.add('selected');
    };

    if (isDir) {
      row.ondblclick = () => enterDirectory(entry.name);
    }

    row.innerHTML = `
      <td class="${typeClass}">${typeLabel}</td>
      <td style="font-weight: ${isDir ? 'bold' : 'normal'};">${escapeHTML(entry.name)}</td>
      <td>${sizeStr}</td>
      <td>${dateStr}</td>
      <td>
        ${isDir 
          ? `<button class="retro-btn" onclick="enterDirectory('${escapeHTML(entry.name)}')">Open</button>`
          : `<button class="retro-btn" onclick="downloadFile('${escapeHTML(entry.name)}')">Download</button>`
        }
      </td>
    `;
    tbody.appendChild(row);
  });
}

// Navigation helpers
function enterDirectory(dirName) {
  if (currentPath === '.' || currentPath === '') {
    currentPath = dirName;
  } else {
    currentPath = `${currentPath}/${dirName}`;
  }
  document.getElementById('path-input').value = currentPath;
  loadDirectory(currentDevice, currentPath);
}

function navigateUp() {
  if (currentPath === '.' || currentPath === '' || currentPath === '/') return;
  const parts = currentPath.split('/').filter(p => p.length > 0);
  parts.pop();
  currentPath = parts.length === 0 ? '.' : parts.join('/');
  document.getElementById('path-input').value = currentPath;
  loadDirectory(currentDevice, currentPath);
}

function handlePathKey(e) {
  if (e.key === 'Enter') navigateToInputPath();
}

function navigateToInputPath() {
  currentPath = document.getElementById('path-input').value.trim() || '.';
  loadDirectory(currentDevice, currentPath);
}

function refreshCurrentView() {
  loadDirectory(currentDevice, currentPath);
  loadStatus();
  appendLog(`[R] Directory refreshed.`);
}

// File Download
function downloadFile(fileName) {
  const filePath = currentPath === '.' ? fileName : `${currentPath}/${fileName}`;
  const url = `/api/download?peer=${encodeURIComponent(currentDevice)}&path=${encodeURIComponent(filePath)}`;
  
  appendLog(`[DL] Downloading '${fileName}'...`);
  const link = document.createElement('a');
  link.href = url;
  link.download = fileName;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
}

// File Upload Trigger
function triggerFileUpload() {
  document.getElementById('file-input-hidden').click();
}

function handleFileSelected(e) {
  const files = e.target.files;
  if (!files || files.length === 0) return;
  uploadFiles(files);
}

// Drag and Drop Upload Handlers
function handleDragOver(e) {
  e.preventDefault();
  document.getElementById('dropzone-overlay').classList.add('active');
}

function handleDragLeave(e) {
  e.preventDefault();
  document.getElementById('dropzone-overlay').classList.remove('active');
}

function handleDrop(e) {
  e.preventDefault();
  document.getElementById('dropzone-overlay').classList.remove('active');
  const files = e.dataTransfer.files;
  if (files && files.length > 0) {
    uploadFiles(files);
  }
}

// Upload Files via FormData
async function uploadFiles(files) {
  for (let i = 0; i < files.length; i++) {
    const file = files[i];
    appendLog(`[UL] Starting upload for '${file.name}' (${formatBytes(file.size)})...`);

    const formData = new FormData();
    formData.append('file', file);
    formData.append('peer', currentDevice);
    formData.append('target_dir', currentPath);

    try {
      const res = await fetch('/api/upload', {
        method: 'POST',
        body: formData,
      });

      if (!res.ok) {
        const errData = await res.json();
        appendLog(`[ERR] Upload failed for '${file.name}': ${errData.error || 'Server error'}`);
      } else {
        appendLog(`[OK] Uploaded '${file.name}' successfully.`);
      }
    } catch (err) {
      appendLog(`[ERR] Upload error: ${err.message}`);
    }
  }

  loadDirectory(currentDevice, currentPath);
}

// Trigger Delta Sync
async function triggerSync() {
  if (currentDevice === 'local') {
    alert('Please select a remote peer to synchronize with.');
    return;
  }

  appendLog(`[SYNC] Triggering delta directory sync for '${currentPath}'...`);
  try {
    const res = await fetch('/api/sync', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ peer: currentDevice, remote_dir: currentPath, local_dir: './sync_output' }),
    });

    const data = await res.json();
    if (!res.ok) {
      appendLog(`[ERR] Sync error: ${data.error || 'Failed'}`);
    } else {
      appendLog(`[OK] Sync completed: ${data.downloaded} downloaded, ${data.skipped} skipped.`);
      loadDirectory(currentDevice, currentPath);
    }
  } catch (err) {
    appendLog(`[ERR] Sync network error: ${err.message}`);
  }
}

// Transfers Real-Time Monitor
function startTransferPolling() {
  transferPollInterval = setInterval(async () => {
    try {
      const res = await fetch('/api/transfers');
      if (!res.ok) return;
      activeTransfers = await res.json();
      
      const badge = document.getElementById('transfers-count-badge');
      if (activeTransfers.length > 0) {
        badge.textContent = `(${activeTransfers.length})`;
      } else {
        badge.textContent = '';
      }

      if (document.getElementById('tab-transfers').classList.contains('active')) {
        renderTransfers();
      }
    } catch (_) {}
  }, 1000);
}

function renderTransfers() {
  const container = document.getElementById('transfers-list');
  if (!container) return;

  if (activeTransfers.length === 0) {
    container.innerHTML = `<div style="text-align: center; padding: 32px; color: var(--text-muted); font-family: var(--font-mono);">No active transfers in queue.</div>`;
    return;
  }

  container.innerHTML = '';
  activeTransfers.forEach(t => {
    const percent = t.total > 0 ? (t.transferred / t.total) * 100 : 0;
    const card = document.createElement('div');
    card.className = 'transfer-card';
    card.innerHTML = `
      <div class="transfer-header">
        <span>[${t.direction === 'upload' ? 'PUSH' : 'PULL'}] ${escapeHTML(t.filename)}</span>
        <span style="color: var(--text-cyan);">${percent.toFixed(1)}%</span>
      </div>
      <div class="progress-bar-container">
        <div class="progress-bar-fill" style="width: ${percent}%;"></div>
        <div class="progress-bar-text">${formatBytes(t.transferred)} / ${formatBytes(t.total)}</div>
      </div>
      <div class="transfer-meta">
        <span>Speed: ${(t.speed / (1024 * 1024)).toFixed(2)} MB/s</span>
        <span>Peer: ${escapeHTML(t.peer_name || t.peer)}</span>
        <span>Status: ${escapeHTML(t.status)}</span>
      </div>
    `;
    container.appendChild(card);
  });
}

// Modal Handlers
function showAddPeerModal() {
  document.getElementById('modal-add-peer').classList.add('active');
}

function showNewFolderModal() {
  document.getElementById('modal-new-folder').classList.add('active');
}

function showHelpModal() {
  document.getElementById('modal-help').classList.add('active');
}

function closeModals() {
  document.querySelectorAll('.modal-backdrop').forEach(el => el.classList.remove('active'));
}

// Submit Add Peer
async function submitAddPeer() {
  const name = document.getElementById('modal-peer-name').value.trim();
  const id = document.getElementById('modal-peer-id').value.trim();
  const key = document.getElementById('modal-peer-key').value.trim();
  const ip = document.getElementById('modal-peer-ip').value.trim();

  if (!id || !key) {
    alert('Please enter Device ID and Public Key.');
    return;
  }

  try {
    const res = await fetch('/api/peers/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ device_id: id, device_name: name || id.substring(0, 8), public_key: key, virtual_ip: ip }),
    });

    if (res.ok) {
      closeModals();
      loadPeers();
      appendLog(`[OK] Paired peer '${name || id}' successfully.`);
    } else {
      const errData = await res.json();
      alert(`Failed to add peer: ${errData.error}`);
    }
  } catch (err) {
    alert(`Error: ${err.message}`);
  }
}

// Remove Peer
async function removePeer(deviceId) {
  if (!confirm(`Are you sure you want to remove peer ${deviceId}?`)) return;

  try {
    const res = await fetch(`/api/peers/delete?id=${encodeURIComponent(deviceId)}`, { method: 'POST' });
    if (res.ok) {
      loadPeers();
      appendLog(`[OK] Removed peer '${deviceId}'.`);
    }
  } catch (err) {
    appendLog(`[ERR] Failed to remove peer: ${err.message}`);
  }
}

// Security / ACL Management
function populateACLSelect(peers) {
  const select = document.getElementById('acl-peer-select');
  if (!select) return;
  select.innerHTML = '';

  peers.forEach(peer => {
    const opt = document.createElement('option');
    opt.value = peer.device_id;
    opt.textContent = `${peer.device_name} (${peer.device_id.substring(0, 12)}...)`;
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
      body: JSON.stringify({
        peer_id: select.value,
        allow_read: allowRead,
        allow_write: allowWrite,
        allowed_paths: allowed,
        blocked_paths: blocked,
      }),
    });

    if (res.ok) {
      appendLog(`[SEC] Updated ACL policy for peer '${select.value}'.`);
      alert('Access Control Policy updated.');
      loadPeers();
    }
  } catch (err) {
    appendLog(`[ERR] Failed to save ACL: ${err.message}`);
  }
}

// Utility functions
function appendLog(msg) {
  const consoleEl = document.getElementById('console-logs');
  if (!consoleEl) return;
  const time = new Date().toLocaleTimeString();
  consoleEl.innerHTML += `[${time}] ${escapeHTML(msg)}<br>`;
  consoleEl.scrollTop = consoleEl.scrollHeight;
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
