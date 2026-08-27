// Isthmus Modern OLED Black GUI Logic
let currentDevice = 'local';
let currentPath = '.';
let allPeers = [];
let localNode = null;
let activeTransfers = [];
let transferPollInterval = null;

window.addEventListener('DOMContentLoaded', () => {
  loadStatus();
  loadPeers();
  loadDirectory('local', '.');
  loadLogs();
  startTransferPolling();
});

// Tab navigation
function switchTab(tabId) {
  document.querySelectorAll('.tab-content').forEach(el => el.classList.remove('active'));
  document.querySelectorAll('.nav-tab').forEach(el => el.classList.remove('active'));
  
  const panel = document.getElementById(`tab-${tabId}`);
  const navBtn = document.getElementById(`nav-${tabId}`);
  
  if (panel) panel.classList.add('active');
  if (navBtn) navBtn.classList.add('active');

  if (tabId === 'peers') loadPeers();
  if (tabId === 'transfers') renderTransfers();
  if (tabId === 'logs') loadLogs();
}

// Fetch local node status
async function loadStatus() {
  try {
    const res = await fetch('/api/status');
    if (!res.ok) return;
    const data = await res.json();
    localNode = data;

    document.getElementById('sidebar-virtual-ip').textContent = data.virtual_ip || '--';
    document.getElementById('sidebar-sftp-port').textContent = data.sftp_port || '--';
    document.getElementById('sidebar-tunnel-port').textContent = data.listen_port || '--';
    document.getElementById('sidebar-coord-status').textContent = data.coord_server ? 'WAN Connected' : 'Direct LAN';

    document.getElementById('sec-device-name').value = data.device_name || '';
    document.getElementById('sec-device-id').value = data.device_id || '';
    document.getElementById('sec-public-key').value = data.public_key || '';
    document.getElementById('sec-shared-dir').value = data.shared_dir || '';
  } catch (err) {
    appendLog(`[ERR] Status error: ${err.message}`);
  }
}

// Fetch peers
async function loadPeers() {
  try {
    const res = await fetch('/api/peers');
    if (!res.ok) return;
    const peers = await res.json();
    allPeers = peers || [];

    const count = allPeers.length + 1;
    document.getElementById('sidebar-device-count').textContent = `${count} Node${count > 1 ? 's' : ''}`;

    renderSidebarPeers(allPeers);
    renderPeersGrid(allPeers);
    populateACLSelect(allPeers);
  } catch (err) {
    appendLog(`[ERR] Peers error: ${err.message}`);
  }
}

// Render Left Sidebar Device List
function renderSidebarPeers(peers) {
  const container = document.getElementById('device-list');
  const localName = localNode ? localNode.device_name : 'This Machine';
  const localIP = localNode ? localNode.virtual_ip : '10.77.0.1';

  container.innerHTML = `
    <div class="peer-item ${currentDevice === 'local' ? 'active' : ''}" onclick="selectDevice('local', '${localName}')">
      <div class="peer-icon">
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
      </div>
      <div class="peer-info">
        <div class="peer-name">${escapeHTML(localName)}</div>
        <div class="peer-meta">${escapeHTML(localIP)} (Self)</div>
      </div>
      <span class="tag">Local</span>
    </div>
  `;

  peers.forEach(peer => {
    const item = document.createElement('div');
    item.className = `peer-item ${currentDevice === peer.device_id ? 'active' : ''}`;
    item.onclick = () => selectDevice(peer.device_id, peer.device_name);

    let tagClass = 'tag-lan';
    let tagText = 'LAN';
    if (peer.transport_tier === 2) {
      tagClass = 'tag-lan';
      tagText = 'WAN STUN';
    } else if (peer.transport_tier === 3) {
      tagClass = 'tag';
      tagText = 'RELAY';
    }

    item.innerHTML = `
      <div class="peer-icon">
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>
      </div>
      <div class="peer-info">
        <div class="peer-name">${escapeHTML(peer.device_name)}</div>
        <div class="peer-meta">${escapeHTML(peer.virtual_ip || peer.device_id.substring(0, 10))}</div>
      </div>
      <span class="tag ${tagClass}">${tagText}</span>
    `;
    container.appendChild(item);
  });
}

// Render Peer Directory Grid
function renderPeersGrid(peers) {
  const grid = document.getElementById('peers-grid');
  if (!grid) return;

  if (peers.length === 0) {
    grid.innerHTML = `<div style="grid-column: 1/-1; padding: 32px; text-align: center; color: var(--text-muted); background: var(--bg-surface); border: 1px solid var(--border-mid); border-radius: var(--radius-md);">No peer nodes configured yet. Click '[+] Pair Peer' to authorize a device.</div>`;
    return;
  }

  grid.innerHTML = '';
  peers.forEach(peer => {
    const card = document.createElement('div');
    card.className = 'peer-card-modern';
    card.innerHTML = `
      <div class="card-top">
        <div style="display: flex; align-items: center; gap: 8px;">
          <div class="peer-icon">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>
          </div>
          <div>
            <div style="font-size: 13px; font-weight: 600; color: #ffffff;">${escapeHTML(peer.device_name)}</div>
            <div style="font-size: 11px; font-family: var(--font-mono); color: var(--blue-text);">${escapeHTML(peer.virtual_ip || '10.77.0.x')}</div>
          </div>
        </div>
        <span class="tag tag-lan">Tier ${peer.transport_tier || 1}</span>
      </div>

      <div class="card-meta-box">
        <div>Device ID: <span style="color: #ffffff;">${escapeHTML(peer.device_id)}</span></div>
        <div>Public Key: <span style="color: var(--blue-text);">${escapeHTML(peer.public_key.substring(0, 24))}...</span></div>
      </div>

      <div style="display: flex; gap: 8px; margin-top: auto;">
        <button class="btn btn-sm btn-primary" style="flex: 1;" onclick="selectDevice('${peer.device_id}', '${peer.device_name}'); switchTab('explorer');">
          Browse Files
        </button>
        <button class="btn btn-sm" onclick="removePeer('${peer.device_id}')">
          Remove
        </button>
      </div>
    `;
    grid.appendChild(card);
  });
}

// Select active device
function selectDevice(deviceId, deviceName) {
  currentDevice = deviceId;
  currentPath = '.';
  
  const displayName = deviceName || (deviceId === 'local' ? 'This Machine' : deviceId);
  document.getElementById('status-peer-name').textContent = displayName;
  updateBreadcrumbs('.');
  
  renderSidebarPeers(allPeers);
  loadDirectory(deviceId, currentPath);
  appendLog(`[NAV] Switched device to '${displayName}'`);
}

// Load Directory
async function loadDirectory(deviceId, path) {
  const tbody = document.getElementById('file-table-body');
  tbody.innerHTML = `<tr><td colspan="4" style="text-align: center; padding: 32px; color: var(--text-muted);">Reading directory contents...</td></tr>`;

  try {
    const res = await fetch(`/api/browse?peer=${encodeURIComponent(deviceId)}&path=${encodeURIComponent(path)}`);
    if (!res.ok) {
      const errData = await res.json();
      tbody.innerHTML = `<tr><td colspan="4" style="text-align: center; padding: 32px; color: #ff5555;">[ERR] ${escapeHTML(errData.error || 'Access Denied')}</td></tr>`;
      return;
    }

    const data = await res.json();
    renderFileTable(data.entries || []);
    document.getElementById('status-items-count').textContent = `${data.entries ? data.entries.length : 0} items`;
    document.getElementById('status-transport-tier').textContent = data.tier || 'Direct Connection';
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="4" style="text-align: center; padding: 32px; color: #ff5555;">[ERR] Connection error: ${escapeHTML(err.message)}</td></tr>`;
  }
}

// Render File Table Rows
function renderFileTable(entries) {
  const tbody = document.getElementById('file-table-body');
  tbody.innerHTML = '';

  if (entries.length === 0) {
    tbody.innerHTML = `<tr><td colspan="4" style="text-align: center; padding: 40px; color: var(--text-muted);">This folder is empty. Drag and drop files here to upload.</td></tr>`;
    return;
  }

  // Parent directory entry
  if (currentPath !== '.' && currentPath !== '' && currentPath !== '/') {
    const row = document.createElement('tr');
    row.ondblclick = () => navigateUp();
    row.innerHTML = `
      <td>
        <div class="name-cell">
          <span class="file-type-icon">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>
          </span>
          <span style="font-weight: 600;">.. (Parent Directory)</span>
        </div>
      </td>
      <td>--</td>
      <td>--</td>
      <td style="text-align: right;">
        <button class="btn btn-sm" onclick="navigateUp()">Up</button>
      </td>
    `;
    tbody.appendChild(row);
  }

  entries.forEach(entry => {
    const row = document.createElement('tr');
    const isDir = entry.is_dir;
    const sizeStr = isDir ? '--' : formatBytes(entry.size);
    const dateStr = entry.modified ? new Date(entry.modified).toLocaleDateString() + ' ' + new Date(entry.modified).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'}) : '--';

    row.onclick = () => {
      document.querySelectorAll('.file-table tbody tr').forEach(r => r.classList.remove('selected'));
      row.classList.add('selected');
    };

    if (isDir) {
      row.ondblclick = () => enterDirectory(entry.name);
    }

    const iconSvg = isDir
      ? `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="color: var(--blue-text);"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>`
      : `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="color: var(--text-secondary);"><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><polyline points="13 2 13 9 20 9"/></svg>`;

    row.innerHTML = `
      <td>
        <div class="name-cell">
          <span class="file-type-icon">${iconSvg}</span>
          <span>${escapeHTML(entry.name)}</span>
        </div>
      </td>
      <td style="font-family: var(--font-mono); font-size: 11.5px;">${sizeStr}</td>
      <td style="font-size: 11.5px; color: var(--text-muted);">${dateStr}</td>
      <td style="text-align: right; display: flex; gap: 4px; justify-content: flex-end;">
        ${isDir 
          ? `<button class="btn btn-sm" onclick="enterDirectory('${escapeHTML(entry.name)}')">Open</button>`
          : `<button class="btn btn-sm btn-primary" onclick="downloadFile('${escapeHTML(entry.name)}')">Download</button>`
        }
        <button class="btn btn-sm" onclick="deleteItem('${escapeHTML(currentPath === '.' ? entry.name : currentPath + '/' + entry.name)}')">Delete</button>
      </td>
    `;
    tbody.appendChild(row);
  });
}

// Navigation & Breadcrumbs
function updateBreadcrumbs(path) {
  const container = document.getElementById('breadcrumb-container');
  const parts = path === '.' || path === '' ? [] : path.split('/').filter(p => p.length > 0);

  let html = `<span class="crumb-btn" onclick="navigateToPath('.')">SharedRoot</span>`;
  let currentAccum = '';

  parts.forEach((part, index) => {
    currentAccum = currentAccum === '' ? part : `${currentAccum}/${part}`;
    html += ` <span class="crumb-sep">/</span> `;
    if (index === parts.length - 1) {
      html += `<span class="crumb-current">${escapeHTML(part)}</span>`;
    } else {
      const target = currentAccum;
      html += `<span class="crumb-btn" onclick="navigateToPath('${escapeHTML(target)}')">${escapeHTML(part)}</span>`;
    }
  });

  container.innerHTML = html;
}

function navigateToPath(targetPath) {
  currentPath = targetPath;
  updateBreadcrumbs(currentPath);
  loadDirectory(currentDevice, currentPath);
}

function enterDirectory(dirName) {
  currentPath = (currentPath === '.' || currentPath === '') ? dirName : `${currentPath}/${dirName}`;
  updateBreadcrumbs(currentPath);
  loadDirectory(currentDevice, currentPath);
}

function navigateUp() {
  if (currentPath === '.' || currentPath === '' || currentPath === '/') return;
  const parts = currentPath.split('/').filter(p => p.length > 0);
  parts.pop();
  currentPath = parts.length === 0 ? '.' : parts.join('/');
  updateBreadcrumbs(currentPath);
  loadDirectory(currentDevice, currentPath);
}

function refreshCurrentView() {
  loadDirectory(currentDevice, currentPath);
  loadStatus();
  appendLog(`[INFO] Refreshed directory view.`);
}

// File Download
function downloadFile(fileName) {
  const filePath = currentPath === '.' ? fileName : `${currentPath}/${fileName}`;
  const url = `/api/download?peer=${encodeURIComponent(currentDevice)}&path=${encodeURIComponent(filePath)}`;
  
  appendLog(`[DOWNLOAD] Fetching '${fileName}'...`);
  const link = document.createElement('a');
  link.href = url;
  link.download = fileName;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
}

// File Upload
function triggerFileUpload() {
  document.getElementById('file-input-hidden').click();
}

function handleFileSelected(e) {
  const files = e.target.files;
  if (!files || files.length === 0) return;
  uploadFiles(files);
}

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

async function uploadFiles(files) {
  for (let i = 0; i < files.length; i++) {
    const file = files[i];
    appendLog(`[UPLOAD] Streaming '${file.name}' (${formatBytes(file.size)})...`);

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
      appendLog(`[ERR] Upload connection error: ${err.message}`);
    }
  }

  loadDirectory(currentDevice, currentPath);
}

// Delta Sync
async function triggerSync() {
  if (currentDevice === 'local') {
    alert('Please select a remote peer to synchronize with.');
    return;
  }

  appendLog(`[SYNC] Triggering delta sync for path '${currentPath}'...`);
  try {
    const res = await fetch('/api/sync', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ peer: currentDevice, remote_dir: currentPath, local_dir: './sync_output' }),
    });

    const data = await res.json();
    if (!res.ok) {
      appendLog(`[ERR] Sync failed: ${data.error || 'Error'}`);
    } else {
      appendLog(`[SYNC OK] ${data.downloaded} downloaded, ${data.skipped} skipped.`);
      loadDirectory(currentDevice, currentPath);
    }
  } catch (err) {
    appendLog(`[ERR] Sync network error: ${err.message}`);
  }
}

// Real-Time Transfers Monitor
function startTransferPolling() {
  transferPollInterval = setInterval(async () => {
    try {
      const res = await fetch('/api/transfers');
      if (!res.ok) return;
      activeTransfers = await res.json();
      
      const badge = document.getElementById('transfers-badge');
      if (activeTransfers && activeTransfers.length > 0) {
        badge.style.display = 'inline-block';
        badge.textContent = activeTransfers.length;
      } else {
        badge.style.display = 'none';
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

  if (!activeTransfers || activeTransfers.length === 0) {
    container.innerHTML = `<div style="text-align: center; padding: 40px; color: var(--text-muted);">No active transfers in queue.</div>`;
    return;
  }

  container.innerHTML = '';
  activeTransfers.forEach(t => {
    const percent = t.total > 0 ? (t.transferred / t.total) * 100 : 0;
    const row = document.createElement('div');
    row.className = 'transfer-row';
    row.innerHTML = `
      <div style="display: flex; justify-content: space-between; font-size: 12.5px; font-weight: 600;">
        <span>${escapeHTML(t.filename)}</span>
        <span style="color: var(--blue-text); font-family: var(--font-mono);">${percent.toFixed(1)}%</span>
      </div>
      <div class="progress-track">
        <div class="progress-fill" style="width: ${percent}%;"></div>
      </div>
      <div style="display: flex; justify-content: space-between; font-size: 11px; font-family: var(--font-mono); color: var(--text-muted);">
        <span>${formatBytes(t.transferred)} / ${formatBytes(t.total)}</span>
        <span>Speed: ${(t.speed / (1024 * 1024)).toFixed(2)} MB/s</span>
        <span style="color: var(--text-secondary); text-transform: uppercase;">${escapeHTML(t.status)}</span>
      </div>
    `;
    container.appendChild(row);
  });
}

// Modal Handlers
function showAddPeerModal() {
  document.getElementById('modal-add-peer').classList.add('active');
}

function showNewFolderModal() {
  document.getElementById('modal-new-folder').classList.add('active');
}

async function submitNewFolder() {
  const folderName = document.getElementById('modal-folder-name').value.trim();
  if (!folderName) {
    alert('Please enter a folder name.');
    return;
  }

  try {
    const res = await fetch('/api/mkdir', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        peer: currentDevice,
        current_dir: currentPath,
        folder_name: folderName,
      }),
    });

    if (res.ok) {
      closeModals();
      document.getElementById('modal-folder-name').value = '';
      loadDirectory(currentDevice, currentPath);
      appendLog(`[OK] Created folder '${folderName}'.`);
    } else {
      const errData = await res.json();
      alert(`Failed to create folder: ${errData.error}`);
    }
  } catch (err) {
    alert(`Error: ${err.message}`);
  }
}

async function deleteItem(itemPath) {
  if (!confirm(`Are you sure you want to delete '${itemPath}'?`)) return;

  try {
    const res = await fetch('/api/delete', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        peer: currentDevice,
        path: itemPath,
      }),
    });

    if (res.ok) {
      loadDirectory(currentDevice, currentPath);
      appendLog(`[OK] Deleted '${itemPath}'.`);
    } else {
      const errData = await res.json();
      alert(`Failed to delete: ${errData.error}`);
    }
  } catch (err) {
    alert(`Error: ${err.message}`);
  }
}

function closeModals() {
  document.querySelectorAll('.modal-overlay').forEach(el => el.classList.remove('active'));
}

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
      appendLog(`[PEER OK] Paired '${name || id}' successfully.`);
    } else {
      const errData = await res.json();
      alert(`Failed to add peer: ${errData.error}`);
    }
  } catch (err) {
    alert(`Error: ${err.message}`);
  }
}

async function removePeer(deviceId) {
  if (!confirm(`Are you sure you want to remove peer ${deviceId}?`)) return;

  try {
    const res = await fetch(`/api/peers/delete?id=${encodeURIComponent(deviceId)}`, { method: 'POST' });
    if (res.ok) {
      loadPeers();
      appendLog(`[PEER RM] Removed '${deviceId}'.`);
    }
  } catch (err) {
    appendLog(`[ERR] Failed to remove peer: ${err.message}`);
  }
}

// Security & ACL
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
      appendLog(`[SEC OK] Saved ACL policy for '${select.value}'.`);
      alert('Security policy saved.');
      loadPeers();
    }
  } catch (err) {
    appendLog(`[ERR] Failed to save ACL: ${err.message}`);
  }
}

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
