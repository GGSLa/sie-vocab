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

// ── TTS 本地朗读（带语音轮换） ──
// 策略：尝试设 voice 轮换，失败的语音自动加入黑名单持久化到 localStorage
// Edge 能正常轮换，Chrome（中文 Win）会自动发现所有语音都坏 → 回退默认

const GOOD_VOICES = [
    'Microsoft David', 'Microsoft Zira', 'Microsoft Mark',
    'Samantha', 'Alex', 'Daniel', 'Karen', 'Moira', 'Fiona', 'Veena', 'Tom', 'Susan',
    'Google US English', 'Google UK English Female', 'Google UK English Male',
];

// 持久化黑名单（存到 localStorage）
function _getBrokenVoices() {
    try { return new Set(JSON.parse(localStorage.getItem('sie_broken_voices') || '[]')); }
    catch { return new Set(); }
}
function _saveBrokenVoices(set) {
    localStorage.setItem('sie_broken_voices', JSON.stringify([...set]));
}

// 获取可用英文语音列表（每次取新鲜 voice 对象）
function _getWorkingVoices() {
    const broken = _getBrokenVoices();
    const all = speechSynthesis.getVoices().filter(v => v.lang && v.lang.startsWith('en') && !broken.has(v.name));
    const good = all.filter(v => GOOD_VOICES.some(name => v.name.includes(name)));
    const source = good.length > 0 ? good : all;
    return source;
}

// 轮换选语音
function _pickVoice() {
    const voices = _getWorkingVoices();
    if (voices.length === 0) return null;
    const cur = parseInt(localStorage.getItem('sie_tts_voice_idx') || '0');
    const next = (cur + 1) % voices.length;
    localStorage.setItem('sie_tts_voice_idx', next);
    return voices[cur];
}

// 标记语音为坏
function _markBroken(name) {
    const broken = _getBrokenVoices();
    broken.add(name);
    _saveBrokenVoices(broken);
    // 重置轮换索引，下次从新列表开始
    localStorage.setItem('sie_tts_voice_idx', '0');
}

function createUtterance(text) {
    const u = new SpeechSynthesisUtterance(text);
    u.rate = 0.85;
    // Chrome 中文 Win 上设 lang='en-US' 反而无声，所以仅 Windows 跳过
    if (!/Windows/i.test(navigator.userAgent)) {
        u.lang = 'en-US';
    }
    return u;
}

function safeSpeak(textOrUtterance) {
    const u = typeof textOrUtterance === 'string' ? createUtterance(textOrUtterance) : textOrUtterance;

    // 尝试轮换语音
    const voice = _pickVoice();
    if (voice) u.voice = voice;
    // lang 在 createUtterance 中设为 'en-US'，仅 Windows 跳过（Chrome 中文 Win 设了反而无声）

    window._ttsCurrentUtterance = u;

    const prevOnEnd = u.onend;
    const prevOnError = u.onerror;
    u.onend = () => {
        window._ttsCurrentUtterance = null;
        if (prevOnEnd) prevOnEnd();
    };
    u.onerror = () => {
        // 语音坏了！加入黑名单，让下次用别的
        if (u.voice) _markBroken(u.voice.name);
        window._ttsCurrentUtterance = null;
        if (prevOnError) prevOnError();
    };

    window.speechSynthesis.cancel();
    window.speechSynthesis.speak(u);
}
