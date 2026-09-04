/* VibeChat WebSocket Client */

let chatSocket = null;
let typingTimer = null;
let isTyping = false;
const typingUsers = new Set();

// ─── WebSocket Connection ───────────────────────────────────────
function connectWebSocket() {
  const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${wsProtocol}//${window.location.host}/ws/chat/${ROOM_SLUG}/`;

  chatSocket = new WebSocket(wsUrl);

  chatSocket.onopen = () => {
    updateConnectionStatus(true);
    console.log('[WS] Connected to room:', ROOM_SLUG);
  };

  chatSocket.onclose = (e) => {
    updateConnectionStatus(false);
    console.warn('[WS] Disconnected. Reconnecting in 3s...');
    setTimeout(connectWebSocket, 3000);
  };

  chatSocket.onerror = (err) => {
    console.error('[WS] Error:', err);
  };

  chatSocket.onmessage = (event) => {
    const data = JSON.parse(event.data);
    handleMessage(data);
  };
}

function updateConnectionStatus(connected) {
  const el = document.getElementById('conn-status');
  const text = document.getElementById('conn-text');
  if (!el) return;
  el.className = `connection-status ${connected ? 'connected' : 'disconnected'}`;
  text.textContent = connected ? 'підключено' : 'відключено';
}

// ─── Message Handling ───────────────────────────────────────────
function handleMessage(data) {
  switch (data.type) {
    case 'message':
      appendMessage(data);
      break;
    case 'typing':
      handleTypingIndicator(data);
      break;
    case 'user_status':
      handleUserStatus(data);
      break;
    case 'reaction':
      handleReactionUpdate(data);
      break;
  }
}

function appendMessage(data) {
  const container = document.getElementById('messages');
  const isOwn = data.author === CURRENT_USER;

  const group = document.createElement('div');
  group.className = `message-group ${isOwn ? 'own' : ''} message-new`;
  group.dataset.msgId = data.message_id;

  const avatarHtml = isOwn ? '' : `
    <div class="message-avatar">
      <div class="avatar avatar-sm" style="background: linear-gradient(135deg, #7c6bff, #5e81f4);">
        ${data.initials || data.author[0].toUpperCase()}
      </div>
    </div>
  `;

  group.innerHTML = `
    ${avatarHtml}
    <div class="messages-content">
      <div class="message-meta">
        <span class="message-author">${escapeHtml(data.author)}</span>
        <span class="message-time">${data.timestamp}</span>
      </div>
      <div class="message-bubble">
        ${escapeHtml(data.content)}
        <div class="reaction-picker">
          ${['👍','❤️','😂','😮','🔥','🎉'].map(e =>
            `<span class="emoji-option" onclick="sendReaction(${data.message_id}, '${e}')">${e}</span>`
          ).join('')}
        </div>
      </div>
      <div class="message-reactions" id="reactions-${data.message_id}"></div>
    </div>
  `;

  // Remove empty state if present
  const emptyState = container.querySelector('.empty-state');
  if (emptyState) emptyState.remove();

  container.appendChild(group);
  scrollToBottom();
}

// ─── Sending ────────────────────────────────────────────────────
function sendMessage() {
  const input = document.getElementById('chat-input');
  const content = input.value.trim();

  if (!content || !chatSocket || chatSocket.readyState !== WebSocket.OPEN) return;

  chatSocket.send(JSON.stringify({
    type: 'message',
    content: content,
  }));

  input.value = '';
  input.style.height = 'auto';
  stopTyping();
}

function sendReaction(messageId, emoji) {
  if (!chatSocket || chatSocket.readyState !== WebSocket.OPEN) return;
  chatSocket.send(JSON.stringify({
    type: 'reaction',
    message_id: messageId,
    emoji: emoji,
  }));
}

// ─── Typing Indicator ───────────────────────────────────────────
function handleTypingStart() {
  if (!isTyping) {
    isTyping = true;
    if (chatSocket && chatSocket.readyState === WebSocket.OPEN) {
      chatSocket.send(JSON.stringify({ type: 'typing', is_typing: true }));
    }
  }
  clearTimeout(typingTimer);
  typingTimer = setTimeout(stopTyping, 2000);
}

function stopTyping() {
  if (isTyping) {
    isTyping = false;
    if (chatSocket && chatSocket.readyState === WebSocket.OPEN) {
      chatSocket.send(JSON.stringify({ type: 'typing', is_typing: false }));
    }
  }
}

function handleTypingIndicator(data) {
  const indicator = document.getElementById('typing-indicator');
  const text = document.getElementById('typing-text');

  if (data.is_typing) {
    typingUsers.add(data.username);
  } else {
    typingUsers.delete(data.username);
  }

  if (typingUsers.size === 0) {
    indicator.style.display = 'none';
  } else {
    const names = [...typingUsers].join(', ');
    text.textContent = `${names} ${typingUsers.size === 1 ? 'друкує' : 'друкують'}...`;
    indicator.style.display = 'flex';
  }
}

// ─── Online Status ──────────────────────────────────────────────
function handleUserStatus(data) {
  const dot = document.getElementById(`status-dot-${data.user_id}`);
  if (dot) {
    if (data.status === 'online') {
      dot.classList.remove('offline');
    } else {
      dot.classList.add('offline');
    }
  }
}

// ─── Reactions ──────────────────────────────────────────────────
function handleReactionUpdate(data) {
  const container = document.getElementById(`reactions-${data.message_id}`);
  if (!container) return;

  // Find existing reaction button for this emoji
  let btn = container.querySelector(`[data-emoji="${data.emoji}"]`);

  if (data.count === 0) {
    if (btn) btn.remove();
    return;
  }

  if (btn) {
    btn.querySelector('span').textContent = data.count;
  } else {
    btn = document.createElement('button');
    btn.className = 'reaction-btn';
    btn.dataset.emoji = data.emoji;
    btn.onclick = () => sendReaction(data.message_id, data.emoji);
    btn.innerHTML = `${data.emoji} <span>${data.count}</span>`;
    container.appendChild(btn);
  }

  // Toggle active if current user
  if (data.user_id === CURRENT_USER_ID) {
    btn.classList.toggle('active');
  }
}

// ─── File Upload ────────────────────────────────────────────────
document.getElementById('file-upload')?.addEventListener('change', async (e) => {
  const file = e.target.files[0];
  if (!file) return;

  const formData = new FormData();
  formData.append('file', file);

  try {
    const response = await fetch(UPLOAD_URL, {
      method: 'POST',
      headers: { 'X-CSRFToken': CSRF_TOKEN },
      body: formData,
    });
    const data = await response.json();

    if (data.success) {
      // Append file message to UI
      const isImage = /\.(jpg|jpeg|png|gif|webp)$/i.test(data.file_name);
      const container = document.getElementById('messages');
      const group = document.createElement('div');
      group.className = 'message-group own message-new';
      group.innerHTML = `
        <div class="messages-content">
          <div class="message-meta" style="flex-direction: row-reverse;">
            <span class="message-author">${escapeHtml(data.author)}</span>
            <span class="message-time">${data.timestamp}</span>
          </div>
          <div class="message-bubble" style="background: linear-gradient(135deg, rgba(124, 107, 255, 0.3), rgba(94, 129, 244, 0.2));">
            ${isImage
              ? `<img src="${data.file_url}" alt="${escapeHtml(data.file_name)}" class="chat-image">`
              : `<a href="${data.file_url}" class="file-message" download>
                   <span class="file-icon">📎</span>
                   <div class="file-info">
                     <div class="file-name">${escapeHtml(data.file_name)}</div>
                     <div class="file-size">Завантажити</div>
                   </div>
                   ⬇️
                 </a>`
            }
          </div>
        </div>
      `;
      container.appendChild(group);
      scrollToBottom();
    }
  } catch (err) {
    console.error('Upload failed:', err);
  }

  e.target.value = '';
});

// ─── Emoji Panel ────────────────────────────────────────────────
document.getElementById('emoji-btn')?.addEventListener('click', (e) => {
  e.stopPropagation();
  const panel = document.getElementById('emoji-panel');
  panel.classList.toggle('open');
});

document.addEventListener('click', () => {
  document.getElementById('emoji-panel')?.classList.remove('open');
});

function insertEmoji(emoji) {
  const input = document.getElementById('chat-input');
  const pos = input.selectionStart;
  const value = input.value;
  input.value = value.slice(0, pos) + emoji + value.slice(pos);
  input.focus();
  input.setSelectionRange(pos + emoji.length, pos + emoji.length);
  document.getElementById('emoji-panel').classList.remove('open');
}

// ─── Input Auto-resize ──────────────────────────────────────────
document.getElementById('chat-input')?.addEventListener('input', function () {
  this.style.height = 'auto';
  this.style.height = Math.min(this.scrollHeight, 120) + 'px';
  handleTypingStart();
});

document.getElementById('chat-input')?.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    sendMessage();
  }
});

document.getElementById('send-btn')?.addEventListener('click', sendMessage);

// ─── Utilities ──────────────────────────────────────────────────
function scrollToBottom() {
  const container = document.getElementById('messages');
  container.scrollTop = container.scrollHeight;
}

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

// ─── Init ───────────────────────────────────────────────────────
connectWebSocket();
scrollToBottom();
