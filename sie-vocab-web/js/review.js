// 状态
let currentMode = 'daily';   // 'daily' | 'free' | 'overview'
let currentWordID = null;
let currentWord = null;
let expanded = false;
let overviewYear = 0;        // 0 = current year
let overviewMonth = 0;       // 0 = current month

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
    document.getElementById('btn-mode-overview').classList.toggle('active', mode === 'overview');

    const overview = document.getElementById('overview-container');
    const card = document.getElementById('review-card');
    const actions = document.getElementById('review-actions');
    const loading = document.getElementById('review-loading');

    if (mode === 'overview') {
        card.style.display = 'none';
        actions.style.display = 'none';
        loading.style.display = 'none';
        overview.style.display = 'block';
        fetchOverviewData();
    } else {
        overview.style.display = 'none';
        fetchRandomWord();
    }
}

// ==================== 随机抽词 ====================

async function fetchRandomWord() {
    const loading = document.getElementById('review-loading');
    const card = document.getElementById('review-card');
    const actions = document.getElementById('review-actions');
    const expandBtn = document.getElementById('btn-expand');
    const detail = document.getElementById('review-detail');

    // 重置展开状态
    expanded = false;
    currentWord = null;
    currentWordID = null;

    const apiPath = currentMode === 'free' ? '/api/review/free/random' : '/api/review/random';

    // 延迟 300ms 才显示 loading — 请求快的话直接无感替换
    let loadingTimer = setTimeout(() => {
        loading.innerHTML = '正在抽取单词…';
        loading.className = 'review-loading';
        loading.style.display = 'flex';
    }, 300);

    try {
        const res = await fetch(apiPath, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        });
        const data = await res.json();

        clearTimeout(loadingTimer);

        if (data.error) {
            loading.style.display = 'none';
            if (data.all_done && currentMode === 'daily') {
                loading.style.display = 'flex';
                loading.innerHTML = '<div class="all-done">' +
                    '<div class="all-done-icon">🎉</div>' +
                    '<div class="all-done-title">今日单词已全部复习完毕</div>' +
                    '<div class="all-done-sub">所有基础词族都已过了一遍，明天再来！<br>或切换到 <strong>自由模式</strong> 继续复习</div>' +
                    '<button class="btn-mode-switch" onclick="switchMode(\'free\')">🆓 切换到自由模式</button>' +
                    '</div>';
            } else {
                loading.innerHTML = '<span class="error-msg">❌ ' + esc(data.error) + '</span>';
                loading.style.display = 'block';
            }
            return;
        }

        currentWordID = data.word_id;
        currentWord = data.word;

        // 显示英文单词
        const wordEl = document.getElementById('review-word-en');
        wordEl.textContent = currentWord.word;
        // 长词自动缩小字号以适应展示区域 / auto-shrink long words to fit
        adjustWordFontSize(wordEl);

        // 构建详情内容（保持隐藏）
        buildDetail(currentWord);
        detail.style.display = 'none';
        document.getElementById('review-stats').style.display = 'none';
        expandBtn.textContent = '🔽 展开详情';

        // 隐藏 loading，确保卡片可见
        loading.style.display = 'none';
        card.style.display = 'block';
        actions.style.display = 'flex';
    } catch (err) {
        clearTimeout(loadingTimer);
        loading.innerHTML = '<span class="error-msg">❌ 请求失败: ' + esc(err.message) + '</span>';
        loading.style.display = 'block';
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
                statsHTML += '<br>⏭ 下次复习：<strong>' + formatReviewDate(result.next_review_date) + '</strong>';
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

// ==================== 工具函数 ====================

function formatReviewDate(raw) {
    if (!raw) return '';
    // Handle ISO 8601: "2026-06-20T00:00:00+08:00" → "2026-06-20"
    const m = String(raw).match(/^(\d{4}-\d{2}-\d{2})/);
    return m ? m[1] : raw;
}

// ==================== 单词朗读 ====================

function speakWord() {
    if (!currentWord || !currentWord.word) return;
    const btn = document.getElementById('btn-speak');
    // Cancel any ongoing speech
    window.speechSynthesis.cancel();

    const utterance = new SpeechSynthesisUtterance(currentWord.word);
    utterance.lang = 'en-US';
    utterance.rate = 0.85;  // slightly slower for clarity

    btn.classList.add('speaking');
    utterance.onend = () => btn.classList.remove('speaking');
    utterance.onerror = () => btn.classList.remove('speaking');

    window.speechSynthesis.speak(utterance);
}

// ==================== 长词字号自适应 ====================

function adjustWordFontSize(el) {
    const container = el.parentElement;  // .review-card, inside .review-box
    // Available width = container width minus word element's own horizontal padding
    const containerWidth = container.clientWidth;
    const elStyle = window.getComputedStyle(el);
    const padLeft = parseFloat(elStyle.paddingLeft) || 0;
    const padRight = parseFloat(elStyle.paddingRight) || 0;
    const maxWidth = containerWidth - padLeft - padRight;

    const maxFontSize = 5;   // rem
    const minFontSize = 2.2; // rem — don't go smaller than this

    // Measure text width at max font size using canvas
    const canvas = document.createElement('canvas');
    const ctx = canvas.getContext('2d');
    const fontFamily = elStyle.fontFamily || 'Inter, sans-serif';
    const fontWeight = elStyle.fontWeight || '900';
    const letterSpacing = parseFloat(elStyle.letterSpacing) || 4;
    ctx.font = fontWeight + ' ' + maxFontSize + 'rem ' + fontFamily;

    const textWidth = ctx.measureText(el.textContent).width + letterSpacing * (el.textContent.length - 1);

    if (textWidth <= maxWidth) {
        el.style.fontSize = maxFontSize + 'rem';
        return;
    }

    // Scale down proportionally
    const ratio = maxWidth / textWidth;
    const newSize = Math.max(minFontSize, maxFontSize * ratio);
    // Round down to 1 decimal for cleaner values
    el.style.fontSize = (Math.floor(newSize * 10) / 10) + 'rem';
}

// ==================== 下一个 ====================

function nextWord() {
    fetchRandomWord();
}

// ==================== 总览模式 ====================

async function fetchOverviewData(year, month) {
    const loading = document.getElementById('review-loading');
    const body = {};

    if (year && month) {
        body.year = year;
        body.month = month;
        overviewYear = year;
        overviewMonth = month;
    } else {
        overviewYear = 0;
        overviewMonth = 0;
    }

    // 300ms delay before showing loading
    let loadingTimer = setTimeout(() => {
        loading.innerHTML = '正在加载总览数据…';
        loading.className = 'review-loading';
        loading.style.display = 'flex';
    }, 300);

    try {
        const res = await fetch('/api/review/overview', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body)
        });
        const data = await res.json();
        clearTimeout(loadingTimer);
        loading.style.display = 'none';

        if (data.error) {
            loading.style.display = 'flex';
            loading.innerHTML = '<span class="error-msg">❌ ' + esc(data.error) + '</span>';
            return;
        }

        renderOverview(data);
    } catch (err) {
        clearTimeout(loadingTimer);
        loading.style.display = 'flex';
        loading.innerHTML = '<span class="error-msg">❌ 请求失败: ' + esc(err.message) + '</span>';
    }
}

function renderOverview(data) {
    document.getElementById('ov-total-words').textContent = data.total_words || 0;
    document.getElementById('ov-total-reviews').textContent = data.total_reviews || 0;
    document.getElementById('ov-streak').textContent = data.streak || 0;
    document.getElementById('ov-today-count').textContent = data.today_reviewed || 0;

    document.getElementById('cal-month-label').textContent = data.year + '年' + data.month + '月';
    overviewYear = data.year;
    overviewMonth = data.month;

    // Build lookup map: date -> day data
    const dayMap = {};
    if (data.monthly_data) {
        data.monthly_data.forEach(function(d) { dayMap[d.date] = d; });
    }

    renderCalendar(data.year, data.month, dayMap);
}

function renderCalendar(year, month, dayMap) {
    var grid = document.getElementById('calendar-grid');
    grid.innerHTML = '';

    var dayNames = ['一', '二', '三', '四', '五', '六', '日'];

    // Header row
    dayNames.forEach(function(name) {
        var hdr = document.createElement('div');
        hdr.className = 'calendar-header';
        hdr.textContent = name;
        grid.appendChild(hdr);
    });

    // Calculate leading empty cells (Monday = 0)
    var firstDay = new Date(year, month - 1, 1).getDay(); // 0=Sun
    var startOffset = firstDay === 0 ? 6 : firstDay - 1;

    for (var i = 0; i < startOffset; i++) {
        var empty = document.createElement('div');
        empty.className = 'calendar-cell empty';
        grid.appendChild(empty);
    }

    // Days in month
    var daysInMonth = new Date(year, month, 0).getDate();

    // Today string (only for current real month)
    var now = new Date();
    var todayStr = (now.getFullYear() === year && now.getMonth() + 1 === month)
        ? pad(now.getDate())
        : null;

    function pad(n) { return n < 10 ? '0' + n : '' + n; }

    for (var d = 1; d <= daysInMonth; d++) {
        var dateStr = year + '-' + pad(month) + '-' + pad(d);
        var cell = document.createElement('div');
        cell.className = 'calendar-cell';

        var dayData = dayMap[dateStr];
        if (dayData) {
            cell.classList.add(dayData.is_completed ? 'completed' : 'has-activity');
        }

        if (todayStr && pad(d) === todayStr) {
            cell.classList.add('today');
        }

        var numSpan = document.createElement('div');
        numSpan.className = 'date-number';
        numSpan.textContent = d;
        cell.appendChild(numSpan);

        if (dayData) {
            var badge = document.createElement('div');
            badge.className = 'review-badge';
            badge.textContent = dayData.review_count + '词';
            cell.appendChild(badge);
        }

        grid.appendChild(cell);
    }
}

function navigateMonth(delta) {
    var y = overviewYear || new Date().getFullYear();
    var m = overviewMonth || new Date().getMonth() + 1;

    m += delta;
    if (m < 1) { m = 12; y--; }
    if (m > 12) { m = 1; y++; }

    fetchOverviewData(y, m);
}
