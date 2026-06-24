// 页面初始化
requireAuth();

const input = document.getElementById('input');
const btn   = document.getElementById('btn');
const resp  = document.getElementById('response');

// 状态
let currentMode = 'new';     // 'new' | 'cached' | 'diff'
let oldWords = null;         // DB 旧数据
let currentWords = null;     // 当前展示的词数据
let currentQueryWord = '';   // 当前查询的单词

input.addEventListener('keydown', e => { if (e.key === 'Enter') send(); });

function esc(s) {
    if (!s) return '';
    return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

// ==================== 主流程 ====================

async function send() {
    const word = input.value.trim();
    if (!word) return;
    currentQueryWord = word;

    btn.disabled = true;
    resp.className = 'response-area loading';
    resp.innerHTML = '正在查询…';

    try {
        // 1. 先查数据库
        const qRes = await apiFetch('/api/word/query', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ word })
        });
        const qData = await qRes.json();

        if (qData.found && qData.data && qData.data.words && qData.data.words.length > 0) {
            // 数据库已有
            oldWords = qData.data.words;
            currentWords = qData.data.words;
            currentMode = 'cached';
            renderCachedView(qData.data.words);
        } else {
            // 数据库没有，调 AI
            await fetchAIAndRender();
        }
    } catch (err) {
        resp.className = 'response-area';
        resp.innerHTML = '<span class="error-msg">❌ 请求失败: ' + esc(err.message) + '</span>';
    } finally {
        btn.disabled = false;
        input.focus();
    }
}

// ==================== AI 翻译 ====================

async function fetchAIAndRender(mode) {
    btn.disabled = true;
    resp.className = 'response-area loading';
    resp.innerHTML = '正在分析单词…';

    try {
        const res = await apiFetch('/api/chat', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ message: currentQueryWord })
        });
        const data = await res.json();
        if (data.error) {
            resp.className = 'response-area';
            resp.innerHTML = '<span class="error-msg">❌ 翻译失败: ' + esc(data.error) + '</span>';
            return;
        }
        const json = parseReplyJSON(data.reply);
        if (!json || !json.words) {
            resp.className = 'response-area';
            resp.textContent = data.reply;
            return;
        }

        if (mode === 'diff' && oldWords) {
            // 比对模式
            currentWords = json.words;
            currentMode = 'diff';
            renderDiffView(oldWords, json.words);
        } else {
            // 新模式
            currentWords = json.words;
            oldWords = null;
            currentMode = 'new';
            renderWords(json.words, 'new');
        }
    } catch (err) {
        resp.className = 'response-area';
        resp.innerHTML = '<span class="error-msg">❌ 请求失败: ' + esc(err.message) + '</span>';
    } finally {
        btn.disabled = false;
        input.focus();
    }
}

function parseReplyJSON(reply) {
    try { return JSON.parse(reply); } catch (e) {}
    const cleaned = reply.replace(/^```(?:json)?\s*\n?/i, '').replace(/\n?```\s*$/, '').trim();
    try { return JSON.parse(cleaned); } catch (e) {}
    return null;
}

// ==================== 渲染：缓存视图 ====================

function renderCachedView(words) {
    resp.className = 'response-area';
    resp.innerHTML = '';

    // 操作栏
    const bar = document.createElement('div');
    bar.className = 'action-bar';
    bar.innerHTML = '<span class="cached-notice"><span class="dot"></span> 来自数据库（已保存）</span>';
    const reBtn = document.createElement('button');
    reBtn.className = 'btn-retranslate';
    reBtn.textContent = '🔄 仍要翻译';
    reBtn.onclick = () => fetchAIAndRender('diff');
    bar.appendChild(reBtn);
    resp.appendChild(bar);

    // 渲染单词卡片（无保存按钮）
    words.forEach(w => appendWordCard(resp, w, 'cached'));
}

// ==================== 渲染：新模式（AI 结果 + 保存按钮） ====================

function renderWords(words, mode) {
    resp.className = 'response-area';
    resp.innerHTML = '';

    // 操作栏 — 保存全部
    if (mode === 'new') {
        const bar = document.createElement('div');
        bar.className = 'action-bar';
        const saveAllBtn = document.createElement('button');
        saveAllBtn.className = 'btn-save-all';
        saveAllBtn.textContent = '💾 保存全部';
        saveAllBtn.onclick = () => saveAll();
        bar.appendChild(saveAllBtn);
        resp.appendChild(bar);
    }

    words.forEach((w, i) => appendWordCard(resp, w, mode, i));
}

// ==================== 渲染：比对视图 ====================

function renderDiffView(oldList, newList) {
    resp.className = 'response-area';
    resp.innerHTML = '';

    // 操作栏 — 保存全部
    const bar = document.createElement('div');
    bar.className = 'action-bar';
    const saveAllBtn = document.createElement('button');
    saveAllBtn.className = 'btn-save-all';
    saveAllBtn.textContent = '💾 保存全部新结果';
    saveAllBtn.onclick = () => saveAll();
    bar.appendChild(saveAllBtn);
    resp.appendChild(bar);

    const oldMap = new Map(oldList.map(w => [w.word, w]));
    const newMap = new Map(newList.map(w => [w.word, w]));

    // 每个词一个比对框
    newList.forEach((w, i) => {
        const old = oldMap.get(w.word);
        const box = document.createElement('div');
        box.className = 'diff-compare-box';

        let headerBadges = '';
        if (!old) {
            headerBadges = '<span class="badge-sm">🆕 新发现</span>';
        } else if (!deepEqual(old, w)) {
            headerBadges = '<span class="diff-changed-badge">有变更</span>';
        }

        // 头部
        box.innerHTML = '<div class="diff-compare-box-header">' +
            '<span class="word-title">' + esc(w.word) + '</span>' +
            headerBadges +
            '</div>';

        // Body — 旧/新并排
        const body = document.createElement('div');
        body.className = 'diff-compare-box-body';

        if (old) {
            // 旧版面板
            const oldPanel = document.createElement('div');
            oldPanel.className = 'diff-panel old';
            oldPanel.innerHTML = '<div class="diff-panel-label">旧版</div>';
            body.appendChild(oldPanel);
            appendWordCard(oldPanel, old, 'none');

            // 分隔线
            const divider = document.createElement('div');
            divider.className = 'diff-divider';
            body.appendChild(divider);
        }

        // 新版面板
        const newPanel = document.createElement('div');
        newPanel.className = 'diff-panel new';
        newPanel.innerHTML = '<div class="diff-panel-label">新版</div>';
        body.appendChild(newPanel);
        appendWordCard(newPanel, w, 'none', i);

        box.appendChild(body);

        // 操作行
        const actions = document.createElement('div');
        actions.className = 'diff-compare-actions';
        actions.innerHTML = '<button class="btn-save" data-idx="' + i + '" onclick="saveWord(' + i + ', this)">💾 保存新版</button>';
        box.appendChild(actions);

        resp.appendChild(box);
    });

    // 旧版中有但新版中没有的
    oldList.forEach(w => {
        if (!newMap.has(w.word)) {
            const box = document.createElement('div');
            box.className = 'diff-compare-box';
            box.innerHTML = '<div class="diff-compare-box-header">' +
                '<span class="word-title">' + esc(w.word) + '</span>' +
                '<span class="diff-only-old-badge">旧版独有（新版已移除）</span>' +
                '</div>';
            const body = document.createElement('div');
            body.className = 'diff-compare-box-body';
            const oldPanel = document.createElement('div');
            oldPanel.className = 'diff-panel old';
            oldPanel.innerHTML = '<div class="diff-panel-label">旧版</div>';
            body.appendChild(oldPanel);
            appendWordCard(oldPanel, w, 'none');
            box.appendChild(body);
            resp.appendChild(box);
        }
    });
}

// ==================== 构建单词卡片 ====================

function appendWordCard(parent, w, mode, index) {
    const card = document.createElement('div');
    card.className = 'word-card';

    let html = '';

    // 头部
    html += '<div class="word-header">';
    html += '<span class="word-name">' + esc(w.word) + '</span>';
    html += '<span class="badge ' + (w.type === '基础词' ? 'badge-base' : 'badge-derived') + '">' + esc(w.type) + '</span>';
    html += '<span class="pos-tag">' + esc(w.pos) + '</span>';
    html += '</div>';

    // 衍生关系
    if (w.type === '衍生词' && w.baseWord) {
        html += '<div class="word-derivation">';
        html += '基础词：<strong>' + esc(w.baseWord) + '</strong>';
        if (w.derivation) html += ' — ' + esc(w.derivation);
        html += '</div>';
    }

    // 释义
    if (w.meanings && w.meanings.length > 0) {
        html += '<table><tr><th>释义</th><td>';
        w.meanings.forEach(m => {
            html += '<span class="domain-tag ' + (m.domain === '金融' ? 'finance' : 'daily') + '">' + esc(m.domain) + '</span>';
            html += esc(m.text) + '<br>';
        });
        html += '</td></tr></table>';
    }

    // 例句
    if (w.examples && w.examples.length > 0) {
        html += '<table class="example-table">';
        html += '<tr><th>例句</th><td></td><td></td></tr>';
        w.examples.forEach((ex, i) => {
            html += '<tr>';
            html += '<td>' + (i + 1) + '.</td>';
            html += '<td>' + esc(ex.en) + '</td>';
            html += '<td>' + esc(ex.zh) + '</td>';
            html += '</tr>';
        });
        html += '</table>';
    }

    // 操作按钮
    if (mode === 'new' || mode === 'diff-save') {
        html += '<div class="word-actions">';
        html += '<button class="btn-save" data-idx="' + index + '" onclick="saveWord(' + index + ', this)">💾 保存此词</button>';
        html += '</div>';
    }

    card.innerHTML = html;
    parent.appendChild(card);
}

// ==================== 保存操作 ====================

async function saveWord(index, btnEl) {
    if (!currentWords || !currentWords[index]) return;
    const word = currentWords[index];

    btnEl.disabled = true;
    btnEl.textContent = '保存中…';

    try {
        const res = await apiFetch('/api/word/save', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(word)
        });
        const data = await res.json();
        if (data.success) {
            btnEl.textContent = '✅ 已保存';
            btnEl.className = 'btn-save saved';
        } else {
            btnEl.textContent = '❌ 失败';
            btnEl.disabled = false;
        }
    } catch (err) {
        btnEl.textContent = '❌ 失败';
        btnEl.disabled = false;
    }
}

async function saveAll() {
    if (!currentWords || currentWords.length === 0) return;

    // 禁用所有保存按钮
    const allBtns = document.querySelectorAll('.btn-save, .btn-save-all');
    allBtns.forEach(b => { b.disabled = true; b.textContent = b.className.includes('save-all') ? '保存中…' : '...'; });

    try {
        const res = await apiFetch('/api/word/save-all', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ words: currentWords })
        });
        const data = await res.json();
        if (data.success) {
            // 所有保存按钮变已保存
            document.querySelectorAll('.btn-save').forEach(b => {
                b.textContent = '✅ 已保存';
                b.className = 'btn-save saved';
                b.disabled = true;
            });
            const saveAllBtn = document.querySelector('.btn-save-all');
            if (saveAllBtn) { saveAllBtn.textContent = '✅ 全部已保存 (' + data.count + ')'; saveAllBtn.disabled = true; }
            // 更新缓存
            oldWords = JSON.parse(JSON.stringify(currentWords));
        }
    } catch (err) {
        alert('保存失败: ' + err.message);
        allBtns.forEach(b => { b.disabled = false; });
    }
}

// ==================== 工具函数 ====================

function deepEqual(a, b) {
    // 简单比较（不递归 — 只比较 meanings 和 examples 的文本）
    const ja = JSON.stringify({ m: a.meanings, e: a.examples, p: a.pos, d: a.derivation });
    const jb = JSON.stringify({ m: b.meanings, e: b.examples, p: b.pos, d: b.derivation });
    return ja === jb;
}
