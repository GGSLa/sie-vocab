// ── 登录/注册页逻辑 ──

let authMode = 'login'; // 'login' | 'register'

// SHA-256 纯 JS 实现（兼容 HTTP 环境，crypto.subtle 只在 HTTPS/localhost 可用）
function sha256(message) {
    function rightRotate(v, n) { return (v >>> n) | (v << (32 - n)); }
    const K = [0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2];
    let msg = unescape(encodeURIComponent(message));
    let ml = msg.length * 8;
    let bytes = [];
    for (let i = 0; i < msg.length; i++) bytes.push(msg.charCodeAt(i));
    bytes.push(0x80);
    while ((bytes.length * 8) % 512 !== 448) bytes.push(0);
    let high = Math.floor(ml / 0x100000000), low = ml >>> 0;
    for (let i = 3; i >= 0; i--) bytes.push((high >>> (i * 8)) & 0xff);
    for (let i = 3; i >= 0; i--) bytes.push((low >>> (i * 8)) & 0xff);
    let H = [0x6a09e667,0xbb67ae85,0x3c6ef372,0xa54ff53a,0x510e527f,0x9b05688c,0x1f83d9ab,0x5be0cd19];
    for (let i = 0; i < bytes.length; i += 64) {
        let W = new Array(64);
        for (let t = 0; t < 16; t++) W[t] = (bytes[i+t*4]<<24)|(bytes[i+t*4+1]<<16)|(bytes[i+t*4+2]<<8)|bytes[i+t*4+3];
        for (let t = 16; t < 64; t++) { let s0 = rightRotate(W[t-15],7)^rightRotate(W[t-15],18)^(W[t-15]>>>3); let s1 = rightRotate(W[t-2],17)^rightRotate(W[t-2],19)^(W[t-2]>>>10); W[t] = (W[t-16]+s0+W[t-7]+s1)>>>0; }
        let [a,b,c,d,e,f,g,h] = H;
        for (let t = 0; t < 64; t++) {
            let S1 = rightRotate(e,6)^rightRotate(e,11)^rightRotate(e,25), ch = (e&f)^(~e&g), temp1 = (h+S1+ch+K[t]+W[t])>>>0;
            let S0 = rightRotate(a,2)^rightRotate(a,13)^rightRotate(a,22), maj = (a&b)^(a&c)^(b&c), temp2 = (S0+maj)>>>0;
            h=g;g=f;f=e;e=(d+temp1)>>>0;d=c;c=b;b=a;a=(temp1+temp2)>>>0;
        }
        H = [H[0]+a,H[1]+b,H[2]+c,H[3]+d,H[4]+e,H[5]+f,H[6]+g,H[7]+h].map(x=>(x>>>0));
    }
    return H.map(x=>x.toString(16).padStart(8,'0')).join('');
}

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
    const hashedPassword = sha256(password);

    try {
        const resp = await fetch(endpoint, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username, password: hashedPassword })
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
