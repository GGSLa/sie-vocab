// ── 认证工具模块 ──
// 提供 token 管理、apiFetch 封装、logout、requireAuth

const BASE_PATH = '/sie';
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
        window.location.href = BASE_PATH + '/login.html';
        // 页面即将跳转，后续代码不会执行
        throw new Error('未登录');
    }
    // API 调用加 BASE_PATH 前缀
    if (url.startsWith('/api/')) {
        url = BASE_PATH + url;
    }
    options.headers = options.headers || {};
    options.headers['Authorization'] = 'Bearer ' + token;
    // 注意：如果 body 是 FormData，不要设置 Content-Type（浏览器会自动设置 multipart boundary）
    const response = await fetch(url, options);
    if (response.status === 401) {
        clearToken();
        window.location.href = BASE_PATH + '/login.html';
        throw new Error('登录已过期');
    }
    return response;
}

// 登出
function logout() {
    clearToken();
    window.location.href = BASE_PATH + '/login.html';
}

// 页面级认证检查 — 在 DOMContentLoaded 时调用
function requireAuth() {
    if (!isAuthenticated()) {
        window.location.href = BASE_PATH + '/login.html';
    }
}

// ── TTS 语音管理 ──

// 精选高质量英文语音白名单（按名称部分匹配）
// 各平台预设，排除 iOS 上的新奇搞怪语音（Albert/Bells/Boing/Whisper 等）
const GOOD_VOICES = [
    // ── Windows ──
    'Microsoft David',   // 男声
    'Microsoft Zira',    // 女声
    'Microsoft Mark',    // 男声

    // ── macOS / iOS ──
    'Samantha',          // 美式女声（默认推荐）
    'Alex',              // 美式男声
    'Daniel',            // 英式男声
    'Karen',             // 澳式女声
    'Moira',             // 爱尔兰女声
    'Fiona',             // 苏格兰女声
    'Veena',             // 印度女声
    'Tom',               // 美式男声
    'Susan',             // 美式女声

    // ── Google / Android ──
    'Google US English',
    'Google UK English Female',
    'Google UK English Male',
];

// 获取白名单中的英文语音
function getGoodEnglishVoices() {
    const all = window.speechSynthesis.getVoices().filter(v => v.lang.startsWith('en'));
    const good = all.filter(v => GOOD_VOICES.some(name => v.name.includes(name)));
    // 如果白名单一个都没命中（罕见），回退到全部英文语音
    return good.length > 0 ? good : all;
}

// 轮换选语音（基于 localStorage 持久化当前位置，每次调用自动推进）
function pickNextVoice() {
    const voices = getGoodEnglishVoices();
    if (voices.length === 0) return null;
    const cur = parseInt(localStorage.getItem('sie_tts_voice_idx') || '0');
    const next = (cur + 1) % voices.length;
    localStorage.setItem('sie_tts_voice_idx', next);
    return voices[cur];  // 返回当前位置的语音，然后推进到下一个
}

// 创建带语音的 Utterance（公共工厂函数，每次自动轮换音色）
function createUtterance(text) {
    const u = new SpeechSynthesisUtterance(text);
    u.lang = 'en-US';
    u.rate = 0.85;
    const voice = pickNextVoice();
    if (voice) u.voice = voice;
    return u;
}
