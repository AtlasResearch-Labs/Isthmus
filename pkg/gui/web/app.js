// Isthmus Modern 2026 GUI Application Logic
let currentDevice = 'local';
let currentPath = '.';
let allPeers = [];
let localNode = null;
let activeTransfers = [];
let transferPollInterval = null;

// Initialize app on DOM ready
window.addEventListener('DOMContentLoaded', () => {
  loadStatus();
  loadPeers();
  loadDirectory('local', '.');
  startTransferPolling();
});

// Tab navigation
function switchTab(tabId) {
  document.querySelectorAll('.view-panel').forEach(el => el.classList.remove('active'));
  document.querySelectorAll('.nav-tab').forEach(el => el.classList.remove('active'));
  
  const content = document.getElementById(`view-${tabId}`);
  const navBtn = document.getElementById(`nav-${tabId}`);
  
  if (content) content.classList.add('active');
  if (navBtn) navBtn.classList.add('active');

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

    document.getElementById('sidebar-virtual-ip').textContent = data.virtual_ip || '10.77.0.1';
    document.getElementById('sidebar-sftp-port').textContent = data.sftp_port || '2222';
    document.getElementById('sidebar-tunnel-port').textContent = data.listen_port || '51820';
    document.getElementById('sidebar-coord-status').textContent = data.coord_server ? 'WAN Connected' : 'Direct LAN';

    // Populate Security Tab
    document.getElementById('sec-device-name').value = data.device_name || '';
    document.getElementById('sec-device-id').value = data.device_id || '';
    document.getElementById('sec-public-key').value = data.public_key || '';
    document.getElementById('sec-shared-dir').value = data.shared_dir || '';
  } catch (err) {
    appendLog(`[ERROR] Failed to load status: ${err.message}`);
  }
}

// Fetch peers list
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
    appendLog(`[ERROR] Failed to load peers: ${err.message}`);
  }
}

// Render Left Sidebar Device List
function renderSidebarPeers(peers) {
  const container = document.getElementById('device-list');
  const localName = localNode ? localNode.device_name : 'This Machine';
  const localIP = localNode ? localNode.virtual_ip : '10.77.0.1';

  container.innerHTML = `
    <div class="peer-card ${currentDevice === 'local' ? 'active' : ''}" onclick="selectDevice('local', '${localName}')">
      <div class="peer-avatar">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
      </div>
      <div class="peer-details">
        <div class="peer-title-row">
          <span class="peer-name">${escapeHTML(localName)}</span>
          <span class="tier-badge tier-local">Local</span>
        </div>
        <div class="peer-ip">${escapeHTML(localIP)} (Self)</div>
      </div>
    </div>
  `;

  peers.forEach(peer => {
    const card = document.createElement('div');
    card.className = `peer-card ${currentDevice === peer.device_id ? 'active' : ''}`;
    card.onclick = () => selectDevice(peer.device_id, peer.device_name);

    let tierClass = 'tier-lan';
    let tierText = 'LAN';
    if (peer.transport_tier === 2) {
      tierClass = 'tier-wan';
      tierText = 'WAN STUN';
    } else if (peer.transport_tier === 3) {
      tierClass = 'tier-relay';
      tierText = 'RELAY';
    }

    card.innerHTML = `
      <div class="peer-avatar" style="color: var(--accent-cyan);">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>
      </div>
      <div class="peer-details">
        <div class="peer-title-row">
          <span class="peer-name">${escapeHTML(peer.device_name)}</span>
          <span class="tier-badge ${tierClass}">${tierText}</span>
        </div>
        <div class="peer-ip">${escapeHTML(peer.virtual_ip || peer.device_id.substring(0, 10))}</div>
      </div>
    `;
    container.appendChild(card);
  });
}

// Render Modern Peers Grid Tab
function renderPeersGrid(peers) {
  const grid = document.getElementById('peers-grid');
  if (!grid) return;

  if (peers.length === 0) {
    grid.innerHTML = `
      <div style="grid-column: 1/-1; padding: 48px; text-align: center; color: var(--text-muted); background: var(--bg-card); border-radius: var(--radius-lg); border: 1px solid var(--border-card);">
        <div style="margin-bottom: 12px; opacity: 0.6;">
          <svg width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>
        </div>
        <p style="font-weight: 500; font-size: 14px;">No remote nodes paired yet</p>
        <p style="font-size: 12.5px; margin-top: 4px;">Click 'Pair New Device' to add an authorized peer.</p>
      </div>
    `;
    return;
  }

  grid.innerHTML = '';
  peers.forEach(peer => {
    const card = document.createElement('div');
    card.className = 'device-card-modern';
    card.innerHTML = `
      <div class="card-header-row">
        <div style="display: flex; align-items: center; gap: 10px;">
          <div class="peer-avatar" style="color: var(--accent-cyan);">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>
          </div>
          <div>
            <div style="font-size: 14px; font-weight: 700; color: #ffffff;">${escapeHTML(peer.device_name)}</div>
            <div style="font-size: 11.5px; font-family: var(--font-mono); color: var(--accent-cyan);">${escapeHTML(peer.virtual_ip || '10.77.0.x')}</div>
          </div>
        </div>
        <span class="tier-badge tier-lan">Authorized</span>
      </div>

      <div class="card-device-meta">
        <div>Device ID: <span style="color: #ffffff;">${escapeHTML(peer.device_id)}</span></div>
        <div>Public Key: <span style="color: var(--accent-cyan);">${escapeHTML(peer.public_key.substring(0, 28))}...</span></div>
        <div>Transport: <span style="color: var(--accent-emerald);">Tier ${peer.transport_tier || 1} (Direct)</span></div>
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

// Select Active Device to Browse
function selectDevice(deviceId, deviceName) {
  currentDevice = deviceId;
  currentPath = '.';
  
  const displayName = deviceName || (deviceId === 'local' ? 'This Machine' : deviceId);
  document.getElementById('status-peer-name').textContent = displayName;
  updateBreadcrumbs('.');
  
  renderSidebarPeers(allPeers);
  loadDirectory(deviceId, currentPath);
  appendLog(`[NAV] Switched focus to '${displayName}'`);
}

// Load Directory
async function loadDirectory(deviceId, path) {
  const tbody = document.getElementById('file-table-body');
  tbody.innerHTML = `<tr><td colspan="4" style="text-align: center; padding: 40px; color: var(--text-muted);">Fetching directory contents...</td></tr>`;

  try {
    const res = await fetch(`/api/browse?peer=${encodeURIComponent(deviceId)}&path=${encodeURIComponent(path)}`);
    if (!res.ok) {
      const errData = await res.json();
      tbody.innerHTML = `<tr><td colspan="4" style="text-align: center; padding: 40px; color: var(--accent-rose); font-weight: 500;">Failed to load folder: ${escapeHTML(errData.error || 'Access Denied')}</td></tr>`;
      return;
    }

    const data = await res.json();
    renderFileTable(data.entries || []);
    document.getElementById('status-items-count').textContent = `${data.entries ? data.entries.length : 0} items`;
    document.getElementById('status-transport-tier').textContent = data.tier || 'Direct Connection';
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="4" style="text-align: center; padding: 40px; color: var(--accent-rose);">Network error: ${escapeHTML(err.message)}</td></tr>`;
  }
}

// Render Modern File Table Rows
function renderFileTable(entries) {
  const tbody = document.getElementById('file-table-body');
  tbody.innerHTML = '';

  if (entries.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td colspan="4" style="text-align: center; padding: 56px 20px; color: var(--text-muted);">
          <div style="margin-bottom: 10px; opacity: 0.5;">
            <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>
          </div>
          <div style="font-weight: 500;">This directory is empty</div>
          <div style="font-size: 12px; margin-top: 4px;">Drag and drop files here to start uploading</div>
        </td>
      </tr>
    `;
    return;
  }

  // Parent dir entry if not root
  if (currentPath !== '.' && currentPath !== '' && currentPath !== '/') {
    const row = document.createElement('tr');
    row.ondblclick = () => navigateUp();
    row.innerHTML = `
      <td>
        <div class="file-name-cell">
          <div class="file-icon-box icon-dir">
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>
          </div>
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
    const dateStr = entry.modified ? new Date(entry.modified).toLocaleDateString() + ' ' + new Date(entry.modified).toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'}) : '--';

    row.onclick = () => {
      document.querySelectorAll('.modern-table tbody tr').forEach(r => r.classList.remove('selected'));
      row.classList.add('selected');
    };

    if (isDir) {
      row.ondblclick = () => enterDirectory(entry.name);
    }

    const iconSvg = isDir
      ? `<div class="file-icon-box icon-dir"><svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg></div>`
      : `<div class="file-icon-box"><svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><polyline points="13 2 13 9 20 9"/></svg></div>`;

    row.innerHTML = `
      <td>
        <div class="file-name-cell">
          ${iconSvg}
          <span>${escapeHTML(entry.name)}</span>
        </div>
      </td>
      <td style="font-family: var(--font-mono); font-size: 12px;">${sizeStr}</td>
      <td style="font-size: 12px; color: var(--text-muted);">${dateStr}</td>
      <td style="text-align: right;">
        ${isDir 
          ? `<button class="btn btn-sm" onclick="enterDirectory('${escapeHTML(entry.name)}')">Open</button>`
          : `<button class="btn btn-sm btn-primary" onclick="downloadFile('${escapeHTML(entry.name)}')">Download</button>`
        }
      </td>
    `;
    tbody.appendChild(row);
  });
}

// Navigation & Breadcrumbs
function updateBreadcrumbs(path) {
  const container = document.getElementById('breadcrumb-container');
  const parts = path === '.' || path === '' ? [] : path.split('/').filter(p => p.length > 0);

  let html = `<span class="crumb-item" onclick="navigateToPath('.')">SharedRoot</span>`;
  let currentAccum = '';

  parts.forEach((part, index) => {
    currentAccum = currentAccum === '' ? part : `${currentAccum}/${part}`;
    html += ` <span class="crumb-separator">/</span> `;
    if (index === parts.length - 1) {
      html += `<span class="crumb-active">${escapeHTML(part)}</span>`;
    } else {
      const target = currentAccum;
      html += `<span class="crumb-item" onclick="navigateToPath('${escapeHTML(target)}')">${escapeHTML(part)}</span>`;
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

// Drag & Drop Handlers
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

// Upload Files via FormData
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
        appendLog(`[ERROR] Upload failed for '${file.name}': ${errData.error || 'Server error'}`);
      } else {
        appendLog(`[SUCCESS] Uploaded '${file.name}' successfully.`);
      }
    } catch (err) {
      appendLog(`[ERROR] Upload connection error: ${err.message}`);
    }
  }

  loadDirectory(currentDevice, currentPath);
}

// Trigger Delta Sync
async function triggerSync() {
  if (currentDevice === 'local') {
    alert('Please select a remote mesh peer to synchronize with.');
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
      appendLog(`[ERROR] Sync failed: ${data.error || 'Network error'}`);
    } else {
      appendLog(`[SYNC COMPLETE] ${data.downloaded} files transferred, ${data.skipped} skipped.`);
      loadDirectory(currentDevice, currentPath);
    }
  } catch (err) {
    appendLog(`[ERROR] Sync error: ${err.message}`);
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

      if (document.getElementById('view-transfers').classList.contains('active')) {
        renderTransfers();
      }
    } catch (_) {}
  }, 1000);
}

function renderTransfers() {
  const container = document.getElementById('transfers-list');
  if (!container) return;

  if (!activeTransfers || activeTransfers.length === 0) {
    container.innerHTML = `<div style="text-align: center; padding: 48px; color: var(--text-muted);">No active transfers in queue.</div>`;
    return;
  }

  container.innerHTML = '';
  activeTransfers.forEach(t => {
    const percent = t.total > 0 ? (t.transferred / t.total) * 100 : 0;
    const card = document.createElement('div');
    card.className = 'transfer-item-card';
    card.innerHTML = `
      <div style="display: flex; justify-content: space-between; align-items: center;">
        <span style="font-weight: 600; color: #ffffff;">${escapeHTML(t.filename)}</span>
        <span style="font-family: var(--font-mono); font-size: 12px; color: var(--accent-cyan);">${percent.toFixed(1)}%</span>
      </div>
      <div class="transfer-progress-track">
        <div class="transfer-progress-bar" style="width: ${percent}%;"></div>
      </div>
      <div style="display: flex; justify-content: space-between; font-size: 11.5px; font-family: var(--font-mono); color: var(--text-muted);">
        <span>${formatBytes(t.transferred)} / ${formatBytes(t.total)}</span>
        <span>Speed: ${(t.speed / (1024 * 1024)).toFixed(2)} MB/s</span>
        <span style="color: var(--accent-emerald); text-transform: uppercase;">${escapeHTML(t.status)}</span>
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

function closeModals() {
  document.querySelectorAll('.modal-backdrop').forEach(el => el.classList.remove('active'));
}

async function submitAddPeer() {
  const name = document.getElementById('modal-peer-name').value.trim();
  const id = document.getElementById('modal-peer-id').value.trim();
  const key = document.getElementById('modal-peer-key').value.trim();
  const ip = document.getElementById('modal-peer-ip').value.trim();

  if (!id || !key) {
    alert('Please enter both Device ID and Public Key.');
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
      appendLog(`[PEER] Paired new node '${name || id}' successfully.`);
    } else {
      const errData = await res.json();
      alert(`Failed to add peer: ${errData.error}`);
    }
  } catch (err) {
    alert(`Error: ${err.message}`);
  }
}

async function removePeer(deviceId) {
  if (!confirm(`Are you sure you want to remove peer node ${deviceId}?`)) return;

  try {
    const res = await fetch(`/api/peers/delete?id=${encodeURIComponent(deviceId)}`, { method: 'POST' });
    if (res.ok) {
      loadPeers();
      appendLog(`[PEER] Removed node '${deviceId}'.`);
    }
  } catch (err) {
    appendLog(`[ERROR] Failed to remove peer: ${err.message}`);
  }
}

// Security & ACL Management
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
      appendLog(`[SECURITY] Updated ACL policy for peer '${select.value}'.`);
      alert('Security policy saved successfully.');
      loadPeers();
    }
  } catch (err) {
    appendLog(`[ERROR] Failed to save ACL: ${err.message}`);
  }
}

// Utility Functions
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
