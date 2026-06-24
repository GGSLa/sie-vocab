// ── 认证工具模块 ──
// 提供 token 管理、apiFetch 封装、logout、requireAuth

const TOKEN_KEY = 'sie_vocab_token';

// Token 管理
function getToken() { return localStorage.getItem(TOKEN_KEY); }
function setToken(token) { localStorage.setItem(TOKEN_KEY, token); }
function clearToken() { localStorage.removeItem(TOKEN_KEY); }
function isAuthenticated() { return !!getToken(); }

// apiFetch — 带认证头的 fetch 封装
// 自动添加 Authorization: Bearer <token>
// 遇到 401 或无 token 自动跳转登录页（页面跳转会中断 JS 执行）
// 成功时返回 response 对象
async function apiFetch(url, options = {}) {
    const token = getToken();
    if (!token) {
        window.location.href = '/login.html';
        // 页面即将跳转，后续代码不会执行
        throw new Error('未登录');
    }
    options.headers = options.headers || {};
    options.headers['Authorization'] = 'Bearer ' + token;
    // 注意：如果 body 是 FormData，不要设置 Content-Type（浏览器会自动设置 multipart boundary）
    const response = await fetch(url, options);
    if (response.status === 401) {
        clearToken();
        window.location.href = '/login.html';
        throw new Error('登录已过期');
    }
    return response;
}

// 登出
function logout() {
    clearToken();
    window.location.href = '/login.html';
}

// 页面级认证检查 — 在 DOMContentLoaded 时调用
function requireAuth() {
    if (!isAuthenticated()) {
        window.location.href = '/login.html';
    }
}
