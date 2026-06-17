// 状态
let currentMode = 'daily';   // 'daily' | 'free'
let currentWordID = null;
let currentWord = null;
let expanded = false;

// 页面加载时自动抽词
window.addEventListener('DOMContentLoaded', () => {
    fetchRandomWord();
});

function esc(s) {
    if (!s) return '';
    return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

// ==================== 模式切换 ====================

function switchMode(mode) {
    if (currentMode === mode) return;
    currentMode = mode;

    document.getElementById('btn-mode-daily').classList.toggle('active', mode === 'daily');
    document.getElementById('btn-mode-free').classList.toggle('active', mode === 'free');

    fetchRandomWord();
}

// ==================== 随机抽词 ====================

async function fetchRandomWord() {
    const loading = document.getElementById('review-loading');
    const card = document.getElementById('review-card');
    const actions = document.getElementById('review-actions');
    const expandBtn = document.getElementById('btn-expand');
    const detail = document.getElementById('review-detail');

    // 重置状态
    expanded = false;
    currentWord = null;
    currentWordID = null;
    card.style.display = 'none';
    actions.style.display = 'none';
    detail.style.display = 'none';
    document.getElementById('review-stats').style.display = 'none';
    expandBtn.textContent = '🔽 展开详情';
    loading.innerHTML = '正在抽取单词…';
    loading.className = 'review-loading';
    loading.style.display = 'block';

    const apiPath = currentMode === 'free' ? '/api/review/free/random' : '/api/review/random';

    try {
        const res = await fetch(apiPath, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        });
        const data = await res.json();

        if (data.error) {
            if (data.all_done && currentMode === 'daily') {
                loading.innerHTML = '<div class="all-done">' +
                    '<div class="all-done-icon">🎉</div>' +
                    '<div class="all-done-title">今日单词已全部复习完毕</div>' +
                    '<div class="all-done-sub">所有基础词族都已过了一遍，明天再来！<br>或切换到 <strong>自由模式</strong> 继续复习</div>' +
                    '<button class="btn-mode-switch" onclick="switchMode(\'free\')">🆓 切换到自由模式</button>' +
                    '</div>';
            } else {
                loading.innerHTML = '<span class="error-msg">❌ ' + esc(data.error) + '</span>';
            }
            return;
        }

        currentWordID = data.word_id;
        currentWord = data.word;

        // 显示英文单词
        document.getElementById('review-word-en').textContent = currentWord.word;

        // 构建详情内容
        buildDetail(currentWord);

        loading.style.display = 'none';
        card.style.display = 'block';
        actions.style.display = 'flex';
    } catch (err) {
        loading.innerHTML = '<span class="error-msg">❌ 请求失败: ' + esc(err.message) + '</span>';
    }
}

// ==================== 展开/收起 ====================

async function toggleExpand() {
    const detail = document.getElementById('review-detail');
    const stats = document.getElementById('review-stats');
    const btn = document.getElementById('btn-expand');
    expanded = !expanded;

    if (expanded) {
        detail.style.display = 'block';
        btn.textContent = '🔼 收起详情';
        const result = await recordReview();
        if (result) {
            let statsHTML = '📊 本词复习：<strong>' + result.word_count + '</strong> 次';
            if (result.base_count > 0 || (currentWord && currentWord.type === '基础词')) {
                statsHTML += ' ｜ 词族总计：<strong>' + result.base_count + '</strong> 次';
            }
            if (result.next_review_date) {
                statsHTML += '<br>⏭ 下次复习：<strong>' + result.next_review_date + '</strong>';
            }
            stats.style.display = 'block';
            stats.innerHTML = statsHTML;
        }
    } else {
        detail.style.display = 'none';
        stats.style.display = 'none';
        btn.textContent = '🔽 展开详情';
    }
}

// ==================== 构建详情 ====================

function buildDetail(w) {
    const container = document.getElementById('review-detail');
    let html = '';

    html += '<div class="review-word-header">';
    html += '<span class="badge ' + (w.type === '基础词' ? 'badge-base' : 'badge-derived') + '">' + esc(w.type) + '</span>';
    html += '<span class="pos-tag">' + esc(w.pos) + '</span>';
    html += '</div>';

    if (w.type === '衍生词' && w.baseWord) {
        html += '<div class="word-derivation">';
        html += '基础词：<strong>' + esc(w.baseWord) + '</strong>';
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
            html += '<tr>';
            html += '<td>' + (i + 1) + '.</td>';
            html += '<td>' + esc(ex.en) + '</td>';
            html += '<td>' + esc(ex.zh) + '</td>';
            html += '</tr>';
        });
        html += '</table>';
    }

    container.innerHTML = html;
}

// ==================== 记录复习 ====================

async function recordReview() {
    if (!currentWordID) return null;

    const apiPath = currentMode === 'free' ? '/api/review/free/record' : '/api/review/record';

    try {
        const res = await fetch(apiPath, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ word_id: currentWordID })
        });
        const data = await res.json();
        return {
            word_count: data.word_count || 0,
            base_count: data.base_count || 0,
            next_review_date: data.next_review_date || ''
        };
    } catch (err) {
        return null;
    }
}

// ==================== 下一个 ====================

function nextWord() {
    fetchRandomWord();
}
