// ── "我的"页面 JS — me.js ──

let userInfo = null;

document.addEventListener('DOMContentLoaded', () => {
    requireAuth();
    loadUserInfo();
});

async function loadUserInfo() {
    const container = document.getElementById('me-content');
    container.innerHTML = '<div class="review-loading">加载中…</div>';

    try {
        const resp = await apiFetch('/api/user/info');
        if (!resp.ok) {
            container.innerHTML = '<div class="empty-note">❌ 加载失败，请刷新重试</div>';
            return;
        }
        userInfo = await resp.json();
        renderMe();
    } catch (err) {
        console.error('加载用户信息失败:', err);
        container.innerHTML = '<div class="empty-note">❌ 加载失败，请刷新重试</div>';
    }
}

function renderMe() {
    const container = document.getElementById('me-content');
    container.innerHTML = `
        <!-- 用户信息卡片 -->
        <div class="me-info-card">
            <div class="me-avatar">👤</div>
            <div class="me-details">
                <div class="me-username">${escHtml(userInfo.username)}</div>
                <div class="me-regdate">📅 注册于 ${userInfo.created_at}</div>
            </div>
        </div>

        <!-- 邀请区域 -->
        <div class="me-section">
            <div class="me-section-title">📨 邀请新用户 / Invite New User</div>
            <div class="me-section-desc">
                输入一个新用户名，只有被邀请的用户才能注册本网站。
                Enter a username — only invited users can register.
            </div>
            <div class="me-invite-row">
                <input type="text" id="invite-username" class="upload-input"
                       placeholder="输入要邀请的用户名（3-50字符）" minlength="3" maxlength="50"
                       onkeydown="if(event.key==='Enter')inviteUser()">
                <button class="btn-submit" id="btn-invite" onclick="inviteUser()">📨 邀请</button>
            </div>
            <div id="invite-msg" class="me-invite-msg" style="display:none;"></div>
        </div>

        <!-- 退出区域 -->
        <div class="me-section me-logout-section">
            <button class="btn-logout" onclick="handleLogout()">🚪 退出登录 / Logout</button>
        </div>
    `;
}

async function inviteUser() {
    const input = document.getElementById('invite-username');
    const btn = document.getElementById('btn-invite');
    const msg = document.getElementById('invite-msg');
    const username = input.value.trim();

    if (!username) {
        showInviteMsg('请输入用户名', 'error');
        return;
    }
    if (username.length < 3 || username.length > 50) {
        showInviteMsg('用户名需要 3-50 个字符', 'error');
        return;
    }

    btn.disabled = true;
    btn.textContent = '⏳ 发送中…';
    msg.style.display = 'none';

    try {
        const resp = await apiFetch('/api/invite', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username })
        });
        const data = await resp.json();

        if (data.success) {
            showInviteMsg(`✅ 已邀请「${data.username}」，对方现在可以注册了！`, 'success');
            input.value = '';
        } else {
            showInviteMsg('❌ ' + (data.error || '邀请失败'), 'error');
        }
    } catch (err) {
        console.error('邀请失败:', err);
        showInviteMsg('❌ 网络错误，请重试', 'error');
    }

    btn.disabled = false;
    btn.textContent = '📨 邀请';
}

function showInviteMsg(text, type) {
    const msg = document.getElementById('invite-msg');
    msg.textContent = text;
    msg.className = 'me-invite-msg me-invite-' + type;
    msg.style.display = 'block';
}

function handleLogout() {
    if (confirm('确定要退出登录吗？\nAre you sure you want to logout?')) {
        clearToken();
        window.location.href = BASE_PATH + '/login.html';
    }
}

// ── HTML 转义 ──
function escHtml(s) {
    if (!s) return '';
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#x27;');
}
