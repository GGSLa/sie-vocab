// 状态
let currentMode = 'daily';   // 'daily' | 'free' | 'overview'
let currentWordID = null;
let currentWord = null;
let expanded = false;
let overviewYear = 0;        // 0 = current year
let overviewMonth = 0;       // 0 = current month

// 页面加载时检查认证并自动抽词
window.addEventListener('DOMContentLoaded', () => {
    requireAuth();
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
        const res = await apiFetch(apiPath, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        });
        const data = await res.json();

        clearTimeout(loadingTimer);

        // 批次完成（非错误）
        if (data.batch_done) {
            loading.style.display = 'none';
            card.style.display = 'none';
            actions.style.display = 'none';
            loading.style.display = 'flex';
            loading.innerHTML = '<div class="all-done">' +
                '<div class="all-done-icon">🎉</div>' +
                '<div class="all-done-title">本批 30 词已完成</div>' +
                '<div class="all-done-sub">' + (data.can_more ? '还有单词可以复习，要来一批吗？' : '所有单词都已复习完毕，明天再来！') + '</div>' +
                (data.can_more ? '<button class="btn-mode-switch" onclick="requestNextBatch()">📦 再来一批</button>' : '') +
                '</div>';
            return;
        }

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

        // 隐藏 loading，确保卡片可见（操作按钮等展开后才显示）
        loading.style.display = 'none';
        card.style.display = 'block';
        actions.style.display = 'none';
        // 重置每日模式按钮状态
        document.getElementById('actions-daily').querySelector('.btn-forgotten').style.display = '';
        document.getElementById('actions-daily').querySelector('.btn-remembered').style.display = '';
        document.getElementById('btn-next-batch').style.display = 'none';
    } catch (err) {
        clearTimeout(loadingTimer);
        loading.innerHTML = '<span class="error-msg">❌ 请求失败: ' + esc(err.message) + '</span>';
        loading.style.display = 'block';
    }
}

// ==================== 展开/收起 ====================

function toggleExpand() {
    const detail = document.getElementById('review-detail');
    const stats = document.getElementById('review-stats');
    const btn = document.getElementById('btn-expand');
    const actions = document.getElementById('review-actions');
    expanded = !expanded;

    if (expanded) {
        detail.style.display = 'block';
        btn.textContent = '🔼 收起详情';
        // 展开后按模式显示不同按钮
        actions.style.display = 'flex';
        document.getElementById('actions-daily').style.display = currentMode === 'free' ? 'none' : '';
        document.getElementById('actions-free').style.display = currentMode === 'free' ? '' : 'none';
    } else {
        detail.style.display = 'none';
        stats.style.display = 'none';
        btn.textContent = '🔽 展开详情';
        // 收起时隐藏操作按钮
        actions.style.display = 'none';
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
            html += '<td><span class="example-en">' + esc(ex.en) + '</span> <span class="btn-speak-inline" onclick="event.stopPropagation(); speakExample(this)" title="朗读例句">🔊</span></td>';
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
        const res = await apiFetch(apiPath, {
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
    window.speechSynthesis.cancel();

    const u = createUtterance(currentWord.word);

    btn.classList.add('speaking');
    u.onend = () => btn.classList.remove('speaking');
    u.onerror = () => btn.classList.remove('speaking');

    window.speechSynthesis.speak(u);
}

function speakExample(el) {
    const enSpan = el.parentElement.querySelector('.example-en');
    if (!enSpan) return;
    const text = enSpan.textContent.trim();
    if (!text) return;
    window.speechSynthesis.cancel();
    const u = createUtterance(text);
    el.classList.add('speaking');
    u.onend = () => el.classList.remove('speaking');
    u.onerror = () => el.classList.remove('speaking');
    window.speechSynthesis.speak(u);
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

// ==================== 记住 / 没记住 ====================

async function markRemembered() {
    // 记录复习 + 跳转下一个
    if (currentWordID) {
        const result = await recordReview();
        if (result) {
            // 短暂显示统计信息
            const stats = document.getElementById('review-stats');
            let statsHTML = '📊 本词复习：<strong>' + result.word_count + '</strong> 次';
            if (result.base_count > 0 || (currentWord && currentWord.type === '基础词')) {
                statsHTML += ' ｜ 词族总计：<strong>' + result.base_count + '</strong> 次';
            }
            if (result.next_review_date) {
                statsHTML += '<br>⏭ 下次复习：<strong>' + formatReviewDate(result.next_review_date) + '</strong>';
            }
            stats.style.display = 'block';
            stats.innerHTML = statsHTML;

            // 批次耗尽 → 显示"再来一批"
            if (result.batch_remaining === 0) {
                document.getElementById('actions-daily').querySelector('.btn-forgotten').style.display = 'none';
                document.getElementById('actions-daily').querySelector('.btn-remembered').style.display = 'none';
                document.getElementById('btn-next-batch').style.display = '';
                return; // 不自动跳下一个，等用户点击"再来一批"
            }
        }
    }
    // 稍作延迟让用户看到统计，然后跳下一个
    setTimeout(() => fetchRandomWord(), 600);
}

function markForgotten() {
    // 不记录，直接跳转下一个
    fetchRandomWord();
}

function nextWord() {
    // 自由模式：直接跳下一个
    fetchRandomWord();
}

async function requestNextBatch() {
    // 每日模式：请求下一批 30 词
    const loading = document.getElementById('review-loading');
    const card = document.getElementById('review-card');
    const actions = document.getElementById('review-actions');
    const btnNext = document.getElementById('btn-next-batch');

    loading.innerHTML = '正在准备新一批单词…';
    loading.className = 'review-loading';
    loading.style.display = 'flex';
    card.style.display = 'none';
    actions.style.display = 'none';

    try {
        const res = await apiFetch('/api/review/next-batch', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        });
        const data = await res.json();

        if (data.all_done || data.error) {
            loading.innerHTML = '<div class="all-done">' +
                '<div class="all-done-icon">🎉</div>' +
                '<div class="all-done-title">所有单词都已复习完毕</div>' +
                '<div class="all-done-sub">今天到此为止，明天再来！<br>或切换到 <strong>自由模式</strong> 继续复习</div>' +
                '<button class="btn-mode-switch" onclick="switchMode(\'free\')">🆓 切换到自由模式</button>' +
                '</div>';
            return;
        }

        // 拿到新单词，走正常渲染流程
        currentWordID = data.word_id;
        currentWord = data.word;

        const wordEl = document.getElementById('review-word-en');
        wordEl.textContent = currentWord.word;
        adjustWordFontSize(wordEl);

        buildDetail(currentWord);
        document.getElementById('review-detail').style.display = 'none';
        document.getElementById('review-stats').style.display = 'none';
        document.getElementById('btn-expand').textContent = '🔽 展开详情';
        expanded = false;

        // 恢复每日模式按钮
        document.getElementById('actions-daily').querySelector('.btn-forgotten').style.display = '';
        document.getElementById('actions-daily').querySelector('.btn-remembered').style.display = '';
        btnNext.style.display = 'none';

        loading.style.display = 'none';
        card.style.display = 'block';
    } catch (err) {
        loading.innerHTML = '<span class="error-msg">❌ 请求失败: ' + esc(err.message) + '</span>';
        loading.style.display = 'block';
    }
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
        const res = await apiFetch('/api/review/overview', {
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
