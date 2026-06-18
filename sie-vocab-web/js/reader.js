// SIE Reader — 按原PDF页划分，跨页段落自动补齐
let currentPage = 67;
let currentChunkIndex = 0;
let pageData = null;
let loading = false;

function esc(s) {
    if (!s) return '';
    return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

window.addEventListener('DOMContentLoaded', () => { loadProgress(); });

// ============ Progress ============

async function loadProgress() {
    showLoading('正在加载进度…');
    try {
        const res = await fetch('/api/reader/progress');
        const data = await res.json();
        currentPage = data.current_page || 67;
        currentChunkIndex = data.current_chunk || 0;
        document.getElementById('reader-subtitle').textContent =
            data.current_section || 'SIE 考试教材';
        await fetchPage(currentPage);
    } catch (err) {
        showError('加载进度失败: ' + esc(err.message));
    }
}

async function saveProgress() {
    try {
        await fetch('/api/reader/progress', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
                current_page: currentPage,
                current_chunk: currentChunkIndex,
                section: pageData ? pageData.section : ''
            })
        });
    } catch (err) {
        console.error('保存进度失败:', err);
    }
}

// ============ Page Fetching ============

async function fetchPage(page) {
    if (loading) return;
    loading = true;
    showLoading('正在加载第 ' + page + ' 页…');
    hideError();
    hideEmpty();
    document.getElementById('reader-main').style.display = 'none';
    document.getElementById('reader-actions').style.display = 'none';

    try {
        const res = await fetch('/api/reader/chunk', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({page: page})
        });
        const data = await res.json();

        if (data.error) {
            if (data.error.includes('无文本') || data.error.includes('空白')) {
                showEmpty(data.error + ' — 点击"下一页"继续');
            } else {
                showError(data.error);
            }
            loading = false;
            return;
        }

        pageData = data;
        currentPage = page;
        if (currentChunkIndex >= data.chunks.length) {
            currentChunkIndex = 0;
        }
        loadPageImage(page);
        renderChunk();
        updatePageNav();
        document.getElementById('reader-main').style.display = 'block';
        document.getElementById('reader-actions').style.display = 'block';
        document.getElementById('reader-subtitle').textContent = data.section || 'SIE 考试教材';
        saveProgress();
    } catch (err) {
        showError('请求失败: ' + esc(err.message));
    } finally {
        loading = false;
        hideLoading();
    }
}

// ============ Chunk Rendering ============

function renderChunk() {
    if (!pageData || !pageData.chunks || pageData.chunks.length === 0) {
        showEmpty('此页无内容');
        return;
    }
    if (currentChunkIndex >= pageData.chunks.length) {
        currentChunkIndex = pageData.chunks.length - 1;
    }
    const chunk = pageData.chunks[currentChunkIndex];

    let sectionHTML = '<h2>' + esc(pageData.section || '') + '</h2>';
    sectionHTML += '<span class="reader-page-num">PDF 第 ' + pageData.page + ' 页';
    if (pageData.page_label) {
        sectionHTML += '（原书第 ' + esc(pageData.page_label) + ' 页）';
    }
    sectionHTML += '</span>';
    document.getElementById('reader-section').innerHTML = sectionHTML;

    document.getElementById('reader-en').innerHTML = formatText(chunk.en);
    document.getElementById('reader-zh').innerHTML = formatText(chunk.zh);
    renderVocab(chunk.vocab);
    renderGrammar(chunk.grammar);

    updatePageNav();
    hideLoading();
}

function formatText(text) {
    if (!text) return '<p class="empty-note">（无内容）</p>';
    return text.split(/\n\n+/).map(p => {
        const trimmed = p.trim();
        if (!trimmed) return '';
        const safe = esc(trimmed);
        const clickable = safe.replace(/\b([a-zA-Z]+(?:'[a-zA-Z]+)?)\b/g,
            '<span class="word-clickable" data-word="$1" onclick="lookupWord(this)" title="点击查词">$1</span>');
        return '<p>' + clickable.replace(/\n/g, '<br>') + '</p>';
    }).join('');
}

function renderVocab(vocab) {
    const container = document.getElementById('reader-vocab');
    if (!vocab || vocab.length === 0) {
        container.innerHTML = '<p class="empty-note">（本段无重点词汇）</p>';
        return;
    }
    let html = '<table class="vocab-table">' +
        '<thead><tr><th>单词</th><th>词性</th><th>释义</th><th>例句</th><th>操作</th></tr></thead><tbody>';
    vocab.forEach((v, i) => {
        html += '<tr>' +
            '<td class="vocab-word">' + esc(v.word) + '</td>' +
            '<td class="vocab-pos">' + esc(v.pos) + '</td>' +
            '<td class="vocab-def">' + esc(v.definition) + '</td>' +
            '<td class="vocab-example">' + esc(v.example || '') + '</td>' +
            '<td><button class="btn-save-small" onclick="saveVocabWord(' + i + ', this)">保存</button></td>' +
            '</tr>';
    });
    html += '</tbody></table>';
    container.innerHTML = html;
}

function renderGrammar(grammar) {
    const container = document.getElementById('reader-grammar');
    if (!grammar || grammar.length === 0) {
        container.innerHTML = '<p class="empty-note">（本段无明显语法点）</p>';
        return;
    }
    let html = '';
    grammar.forEach(g => {
        html += '<div class="grammar-item">' +
            '<div class="grammar-point"><strong>' + esc(g.point) + '</strong></div>' +
            '<div class="grammar-detail">' + esc(g.detail) + '</div>' +
            '</div>';
    });
    container.innerHTML = html;
}

// ============ Navigation ============

function nextChunk() {
    if (!pageData || !pageData.chunks) return;
    if (currentChunkIndex < pageData.chunks.length - 1) {
        currentChunkIndex++;
        renderChunk();
        saveProgress();
    } else {
        nextPage();
    }
}

function prevChunk() {
    if (!pageData) return;
    if (currentChunkIndex > 0) {
        currentChunkIndex--;
        renderChunk();
        saveProgress();
    }
}

function nextPage() {
    currentChunkIndex = 0;
    fetchPage(currentPage + 1);
}

function prevPage() {
    if (currentPage > 1) {
        currentChunkIndex = 0;
        fetchPage(currentPage - 1);
    }
}

function jumpToPage() {
    const input = document.getElementById('input-jump-page');
    const page = parseInt(input.value);
    if (page > 0) {
        currentChunkIndex = 0;
        fetchPage(page);
        input.value = '';
    }
}

function updatePageNav() {
    if (!pageData) return;
    document.getElementById('reader-page-display').textContent = '第 ' + currentPage + ' 页';
    document.getElementById('btn-prev-page').disabled = currentPage <= 1;

    const total = pageData.chunks.length;
    const current = currentChunkIndex + 1;
    document.getElementById('reader-chunk-display').textContent = current + ' / ' + total;
    document.getElementById('btn-prev-chunk').disabled = currentChunkIndex <= 0;
    document.getElementById('btn-next-chunk').disabled = currentChunkIndex >= total - 1;

    const pct = Math.round((currentChunkIndex / Math.max(1, total)) * 100);
    document.getElementById('reader-progress').innerHTML =
        '<div class="progress-bar-bg"><div class="progress-bar-fill" style="width:' + pct + '%"></div></div>' +
        '<span class="progress-text">第 ' + currentPage + ' 页  ' + current + '/' + total + ' 段</span>';
}

// ============ Page Image ============

function loadPageImage(page) {
    const container = document.getElementById('reader-image-container');
    container.innerHTML =
        '<div class="reader-page-img-wrap">' +
        '<div class="reader-page-img-label">第 ' + page + ' 页</div>' +
        '<img src="/api/reader/page-image?page=' + page + '&t=' + Date.now() + '" ' +
        'alt="PDF Page ' + page + '" onerror="this.parentElement.innerHTML=\'<div class=reader-image-placeholder>图片加载失败</div>\'">' +
        '</div>';
}

// ============ Word Lookup Modal ============

async function lookupWord(el) {
    const word = el.getAttribute('data-word');
    if (!word) return;

    const modal = document.getElementById('word-modal');
    const modalBody = document.getElementById('word-modal-body');
    modal.style.display = 'flex';
    modalBody.innerHTML = '<div class="review-loading">正在查询 <strong>' + esc(word) + '</strong>…</div>';

    try {
        const res = await fetch('/api/chat', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({message: word})
        });
        const data = await res.json();
        if (data.error) {
            modalBody.innerHTML = '<div class="error-msg">翻译失败: ' + esc(data.error) + '</div>';
            return;
        }
        const parsed = parseReplyJSON(data.reply);
        if (!parsed || !parsed.words || parsed.words.length === 0) {
            modalBody.innerHTML = '<div class="error-msg">无法解析该单词</div>';
            return;
        }
        modalBody.innerHTML = renderLookupCard(parsed.words);
    } catch (err) {
        modalBody.innerHTML = '<div class="error-msg">请求失败: ' + esc(err.message) + '</div>';
    }
}

function parseReplyJSON(reply) {
    try { return JSON.parse(reply); } catch (e) {}
    let cleaned = reply;
    const start = cleaned.indexOf('{');
    const end = cleaned.lastIndexOf('}');
    if (start >= 0 && end > start) cleaned = cleaned.slice(start, end + 1);
    try { return JSON.parse(cleaned); } catch (e) { return null; }
}

function renderLookupCard(words) {
    let html = '';
    words.forEach((w, idx) => {
        html += '<div class="word-card" style="margin-bottom:16px">';
        html += '<div class="word-header">';
        html += '<span class="word-name">' + esc(w.word) + '</span>';
        html += '<span class="badge ' + (w.type === '基础词' ? 'badge-base' : 'badge-derived') + '">' + esc(w.type) + '</span>';
        html += '<span class="pos-tag">' + esc(w.pos || '') + '</span>';
        html += '</div>';
        if (w.type === '衍生词' && w.baseWord) {
            html += '<div class="word-derivation">基础词：<strong>' + esc(w.baseWord) + '</strong>';
            if (w.derivation) html += ' — ' + esc(w.derivation);
            html += '</div>';
        }
        if (w.meanings && w.meanings.length > 0) {
            html += '<table><tr><th>释义</th><td>';
            w.meanings.forEach(m => {
                html += '<span class="domain-tag ' + (m.domain === '金融' ? 'finance' : 'daily') + '">' + esc(m.domain) + '</span>';
                html += esc(m.text) + '<br>';
            });
            html += '</td></tr></table>';
        }
        if (w.examples && w.examples.length > 0) {
            html += '<table class="example-table">';
            html += '<tr><th>例句</th><td></td><td></td></tr>';
            w.examples.forEach((ex, i) => {
                html += '<tr><td>' + (i + 1) + '.</td><td>' + esc(ex.en) + '</td><td>' + esc(ex.zh) + '</td></tr>';
            });
            html += '</table>';
        }
        html += '<div class="word-actions">';
        html += '<button class="btn-save" onclick="saveLookupWord(' + idx + ', this)" data-word=\'' + JSON.stringify(w) + '\'>保存此词</button>';
        html += '</div>';
        html += '</div>';
    });
    return html;
}

async function saveVocabWord(index, btnEl) {
    if (!pageData || !pageData.chunks || !pageData.chunks[currentChunkIndex]) return;
    const v = pageData.chunks[currentChunkIndex].vocab[index];
    if (!v) return;
    btnEl.disabled = true; btnEl.textContent = '保存中…';
    const wordEntry = {
        word: v.word, type: '基础词', pos: v.pos || '', baseWord: null, derivation: null,
        meanings: [{domain: '金融', text: v.definition}],
        examples: v.example ? [{en: v.example, zh: ''}] : []
    };
    try {
        const res = await fetch('/api/word/save', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(wordEntry)});
        const data = await res.json();
        btnEl.textContent = data.success ? '已保存' : '失败';
        if (data.success) btnEl.className = 'btn-save-small saved'; else btnEl.disabled = false;
    } catch (err) { btnEl.textContent = '失败'; btnEl.disabled = false; }
}

async function saveLookupWord(index, btnEl) {
    let w;
    try { w = JSON.parse(btnEl.getAttribute('data-word')); } catch (e) { btnEl.textContent = '失败'; return; }
    btnEl.disabled = true; btnEl.textContent = '保存中…';
    try {
        const res = await fetch('/api/word/save', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(w)});
        const data = await res.json();
        btnEl.textContent = data.success ? '已保存' : '失败';
        if (data.success) btnEl.className = 'btn-save saved'; else btnEl.disabled = false;
    } catch (err) { btnEl.textContent = '失败'; btnEl.disabled = false; }
}

function closeModal(event) {
    if (event && event.target !== document.getElementById('word-modal')) return;
    document.getElementById('word-modal').style.display = 'none';
}

// ============ UI Helpers ============

function showLoading(msg) {
    const el = document.getElementById('reader-loading');
    el.textContent = msg || '正在加载…';
    el.style.display = 'flex';
}
function hideLoading() { document.getElementById('reader-loading').style.display = 'none'; }
function showError(msg) {
    const el = document.getElementById('reader-error');
    el.innerHTML = '<span class="error-msg">' + esc(msg) + '</span>';
    el.style.display = 'block';
}
function hideError() { document.getElementById('reader-error').style.display = 'none'; }
function showEmpty(msg) {
    const el = document.getElementById('reader-empty');
    el.querySelector('.all-done-sub').textContent = msg || '此页无文本内容';
    el.style.display = 'block';
}
function hideEmpty() { document.getElementById('reader-empty').style.display = 'none'; }
