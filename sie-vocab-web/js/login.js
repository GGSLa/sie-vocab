// ── 登录/注册页逻辑 ──

let authMode = 'login'; // 'login' | 'register'

function setAuthMode(mode) {
    authMode = mode;
    document.getElementById('btn-login-mode').classList.toggle('active', mode === 'login');
    document.getElementById('btn-register-mode').classList.toggle('active', mode === 'register');
    document.getElementById('btn-auth-submit').textContent = mode === 'login' ? '登录' : '注册';
    document.getElementById('auth-title').textContent = mode === 'login' ? '🔐 登录' : '✨ 注册';
    document.getElementById('auth-subtitle').textContent = mode === 'login' ? '登录以继续学习' : '创建新账号开始学习';
    document.getElementById('auth-error').style.display = 'none';
    // 注册模式下显示安全警告
    document.getElementById('auth-warning').style.display = mode === 'register' ? 'block' : 'none';
}

async function handleAuth(event) {
    event.preventDefault();

    const username = document.getElementById('auth-username').value.trim();
    const password = document.getElementById('auth-password').value;
    const errorEl = document.getElementById('auth-error');
    const submitBtn = document.getElementById('btn-auth-submit');

    // 客户端验证
    if (username.length < 3) {
        showError('用户名至少需要 3 个字符');
        return;
    }
    if (password.length < 6) {
        showError('密码至少需要 6 个字符');
        return;
    }

    submitBtn.disabled = true;
    submitBtn.textContent = '处理中...';
    errorEl.style.display = 'none';

    const endpoint = authMode === 'login' ? '/api/auth/login' : '/api/auth/register';

    try {
        const resp = await fetch(endpoint, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username, password })
        });
        const data = await resp.json();

        if (data.error) {
            showError(data.error);
            submitBtn.disabled = false;
            submitBtn.textContent = authMode === 'login' ? '登录' : '注册';
            return;
        }

        if (data.token) {
            setToken(data.token);
            window.location.href = '/';
        }
    } catch (err) {
        showError('网络错误，请检查连接后重试');
        submitBtn.disabled = false;
        submitBtn.textContent = authMode === 'login' ? '登录' : '注册';
    }
}

function showError(msg) {
    const errorEl = document.getElementById('auth-error');
    errorEl.textContent = msg;
    errorEl.style.display = 'block';
}

// 页面加载时，如果已登录则直接跳转首页
if (isAuthenticated()) {
    window.location.href = '/';
}
