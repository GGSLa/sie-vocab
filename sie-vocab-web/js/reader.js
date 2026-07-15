// SIE Reader — 整页阅读，跨页段落自动补齐
const CALLOUT_CONT_MARKER = ''; // invisible protocol marker for callout continuation
function isCalloutStart(s) { return s.startsWith('»') || s.startsWith(CALLOUT_CONT_MARKER); }
let currentBookId = 1;
let currentBookTitle = '';
let currentPage = 67;
let pageData = null;
let loading = false;
let tocExpanded = false;
let tocData = [];          // hierarchical outline
let tocDataFlat = [];      // fallback: flat cached pages
let tocCachedPages = {};   // page -> bool for cached markers
let lookupMode = 'new';       // 'new' | 'cached' — current modal display mode
let lookupDBWords = null;     // DB word data when in cached mode
let lookupAIWords = null;     // AI word data when in new mode (for save)
let lookupVocabBtnEl = null;  // original vocab table button ref (for updating after diff save)
let lastLookupWord = '';      // 上次查询的单词 / last looked-up word
let lastLookupPanelHTML = ''; // 上次面板HTML缓存 / cached panel HTML for quick restore
let activePanel = 'pdf';      // 'pdf' | 'word' | 'breakdown' — current left panel mode
let lastBreakdownHTML = '';   // 上次句子拆解HTML缓存 / cached breakdown HTML
let lastBreakdownSentence = ''; // 上次拆解的句子文本

function esc(s) {
    if (!s) return '';
    return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

window.addEventListener('DOMContentLoaded', () => { requireAuth(); loadProgress(); initBreakdownUI(); });

// ============ Progress ============

async function loadProgress() {
    showLoading('正在加载进度…');
    try {
        // Check URL params for book and page (e.g. ?book=1&page=100)
        const urlParams = new URLSearchParams(window.location.search);
        const urlPage = parseInt(urlParams.get('page'));
        const urlBook = parseInt(urlParams.get('book'));

        if (urlBook && urlBook > 0) {
            // Explicit book in URL — use it directly
            currentBookId = urlBook;
        } else {
            // No book specified — ask server for default (last read > first book)
            const res = await apiFetch('/api/reader/last-book');
            const data = await res.json();
            if (data.no_books) {
                showNoBooks();
                return;
            }
            currentBookId = data.book_id || 1;
        }

        if (urlPage && urlPage > 0) {
            // Shared link — override saved progress with URL page
            currentPage = urlPage;
        } else {
            const res = await apiFetch('/api/reader/progress?book=' + currentBookId);
            const data = await res.json();
            currentPage = data.current_page || 1;
            currentBookId = data.book_id || currentBookId;
            document.getElementById('reader-subtitle').textContent =
                data.current_section || 'SIE 考试教材';
        }
        // Load book info and TOC (now after bookId is resolved, fixing race condition)
        loadBookInfo();
        loadToc();
        await fetchPage(currentPage);
    } catch (err) {
        showError('加载进度失败: ' + esc(err.message));
    }
}

async function loadBookInfo() {
    try {
        const res = await apiFetch('/api/books?id=' + currentBookId);
        const data = await res.json();
        if (data && data.book) {
            currentBookTitle = data.book.title;
            document.getElementById('reader-subtitle').textContent = currentBookTitle;
        } else if (data && data.title) {
            // Direct book response
            currentBookTitle = data.title;
            document.getElementById('reader-subtitle').textContent = currentBookTitle;
        }
    } catch (err) {
        console.error('加载书名失败:', err);
    }
}

// Sync browser URL with current page (replaceState, no history push)
function syncPageURL() {
    const url = new URL(window.location);
    url.searchParams.set('book', currentBookId);
    if (url.searchParams.get('page') != currentPage) {
        url.searchParams.set('page', currentPage);
    }
    window.history.replaceState(null, '', url.toString());
}

async function saveProgress() {
    try {
        await apiFetch('/api/reader/progress', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
                book_id: currentBookId,
                current_page: currentPage,
                current_chunk: 0,
                section: pageData ? pageData.section : ''
            })
        });
    } catch (err) {
        console.error('保存进度失败:', err);
    }
}

// ============ TOC Sidebar ============

function toggleToc() {
    const sidebar = document.getElementById('reader-toc-sidebar');
    tocExpanded = !tocExpanded;
    if (tocExpanded) {
        sidebar.classList.add('expanded');
    } else {
        sidebar.classList.remove('expanded');
    }
}

async function loadToc() {
    try {
        const res = await apiFetch('/api/reader/toc?book=' + currentBookId);
        const data = await res.json();
        if (data.outline && data.outline.length > 0) {
            // PDF outline available — render hierarchical TOC
            tocData = data.outline;
            tocCachedPages = data.cached_pages || {};
            renderTocOutline();
        } else if (data.entries) {
            // Fallback: cached pages list
            tocDataFlat = data.entries;
            tocCachedPages = {};
            renderTocFlat();
        } else {
            document.getElementById('reader-toc-list').innerHTML =
                '<div class="reader-toc-empty">目录数据为空</div>';
        }
    } catch (err) {
        console.error('加载目录失败:', err);
        document.getElementById('reader-toc-list').innerHTML =
            '<div class="reader-toc-empty">目录加载失败</div>';
    }
}

function renderTocOutline() {
    const list = document.getElementById('reader-toc-list');
    let html = renderTocItems(tocData, 0);
    list.innerHTML = html || '<div class="reader-toc-empty">无目录条目</div>';
    highlightTocPage();
    // Auto-expand the branch containing the current page
    expandTocToPage(currentPage);
}

// Expand parent containers from the active item up to root
function expandTocToPage(page) {
    const activeItem = document.querySelector('.reader-toc-item[data-page="' + page + '"]');
    if (!activeItem) return;
    let el = activeItem;
    while (el) {
        // For each parent .toc-children, remove collapsed
        if (el.classList.contains('toc-children')) {
            el.classList.remove('collapsed');
            // Also update the arrow of the parent item
            const parentItem = el.previousElementSibling;
            if (parentItem && parentItem.classList.contains('reader-toc-item')) {
                const arrow = parentItem.querySelector('.toc-arrow');
                if (arrow) arrow.textContent = '▼';
            }
        }
        el = el.parentElement;
    }
}

function renderTocItems(items, depth) {
    if (!items || items.length === 0) return '';
    let html = '';
    items.forEach(e => {
        const hasChildren = e.children && e.children.length > 0;
        let cls = 'reader-toc-item';
        if (e.level <= 0) cls += ' toc-level-part';
        else if (e.level === 1) cls += ' toc-level-chapter';
        else cls += ' toc-level-section';
        if (tocCachedPages && tocCachedPages[e.page]) cls += ' toc-cached';
        if (hasChildren) cls += ' toc-has-children';

        html += '<div class="' + cls + '" data-page="' + e.page +
            '" style="padding-left:' + (16 + depth * 14) + 'px"';

        if (hasChildren) {
            // Parent item: arrow toggles children, title + P.N navigate
            html += '>' +
                '<span class="toc-arrow" onclick="event.stopPropagation(); toggleTocChildren(this)">▶</span>';
        } else {
            // Leaf item: click anywhere navigates
            html += ' onclick="jumpToTocPage(' + e.page + ')">';
        }

        html += '<span class="reader-toc-page-num" onclick="event.stopPropagation(); jumpToTocPage(' + e.page + ')">P.' + e.page + '</span>' +
            '<span class="reader-toc-page-title" onclick="event.stopPropagation(); jumpToTocPage(' + e.page + ')">' + esc(e.title) + '</span>';
        if (tocCachedPages && tocCachedPages[e.page]) {
            html += '<span class="toc-dot" title="已读"></span>';
        }
        html += '</div>';

        // Children wrapped in collapsible container (collapsed by default)
        if (hasChildren) {
            html += '<div class="toc-children collapsed">';
            html += renderTocItems(e.children, depth + 1);
            html += '</div>';
        } else if (e.children && e.children.length === 0) {
            // No children — nothing to wrap
        }
    });
    return html;
}

function toggleTocChildren(arrow) {
    const parent = arrow.parentElement;
    const children = parent.nextElementSibling;
    if (!children || !children.classList.contains('toc-children')) return;

    const collapsed = children.classList.toggle('collapsed');
    arrow.textContent = collapsed ? '▶' : '▼';
}

function renderTocFlat() {
    const list = document.getElementById('reader-toc-list');
    if (!tocDataFlat || tocDataFlat.length === 0) {
        list.innerHTML = '<div class="reader-toc-empty">暂无缓存页面 / No cached pages yet</div>';
        return;
    }
    let html = '';
    let lastSection = '';
    tocDataFlat.forEach(e => {
        const section = e.section || '未命名章节';
        if (section !== lastSection) {
            html += '<div class="reader-toc-section-label">' + esc(section) + '</div>';
            lastSection = section;
        }
        html += '<div class="reader-toc-item" data-page="' + e.page + '" onclick="jumpToTocPage(' + e.page + ')">' +
            '<span class="reader-toc-page-num">P.' + e.page + '</span>' +
            '<span class="reader-toc-page-title">第 ' + e.page + ' 页</span>' +
            '</div>';
    });
    list.innerHTML = html;
    highlightTocPage();
}

function highlightTocPage() {
    document.querySelectorAll('.reader-toc-item').forEach(el => {
        el.classList.toggle('active', parseInt(el.getAttribute('data-page')) === currentPage);
    });
    // Also expand the branch containing the current page
    expandTocToPage(currentPage);
}

function jumpToTocPage(page) {
    if (loading) return;
    fetchPage(page);
}

// ============ Page Fetching ============

async function fetchPage(page) {
    if (loading) return;
    loading = true;
    showLoading('正在加载第 ' + page + ' 页…');
    hideError();
    hideEmpty();
    closeWordPanel(false);  // 翻页时恢复PDF视图，清除返回按钮 / restore PDF view, clear return btn
    lastLookupWord = '';
    lastLookupPanelHTML = '';
    lastBreakdownHTML = '';
    lastBreakdownSentence = '';
    hideBreakdownBtn();
    switchLeftPanel('pdf');
    document.getElementById('reader-return-word').style.display = 'none';
    document.getElementById('reader-main').style.display = 'none';
    // Clear breakdown panel content
    document.getElementById('reader-breakdown-panel-body').innerHTML = '';

    // 立即加载左侧 PDF 图片，不等待右侧 chunk API 返回 / Load PDF image immediately; don't wait for chunk API
    loadPageImage(page);

    try {
        const res = await apiFetch('/api/reader/chunk', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({book_id: currentBookId, page: page})
        });
        const data = await res.json();

        if (data.error) {
            // Still update current page so TOC highlight follows
            currentPage = page;
            highlightTocPage();
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
        renderPage();
        updatePageNav();
        highlightTocPage();
        document.getElementById('reader-main').style.display = 'block';
        document.getElementById('reader-subtitle').textContent = data.section || 'SIE 考试教材';
        syncPageURL();  // keep URL in sync with current page for sharing
        saveProgress();
        // Refresh TOC in case a new page was cached
        loadToc();
        // Preload next page in background for instant navigation
        preloadNextPage(page + 1);
    } catch (err) {
        showError('请求失败: ' + esc(err.message));
    } finally {
        loading = false;
        hideLoading();
    }
}

// ============ Page Rendering ============

function renderPage() {
    if (!pageData || !pageData.chunks || pageData.chunks.length === 0) {
        showEmpty('此页无内容');
        return;
    }

    // Merge all chunks into one display
    var allEn = [], allZh = [], allVocab = [], allGrammar = [];
    for (var i = 0; i < pageData.chunks.length; i++) {
        var c = pageData.chunks[i];
        if (c.en) allEn.push(c.en);
        if (c.zh) allZh.push(c.zh);
        if (c.vocab) allVocab = allVocab.concat(c.vocab);
        if (c.grammar) allGrammar = allGrammar.concat(c.grammar);
    }
    var mergedEn = allEn.join('\n\n');
    var mergedZh = allZh.join('\n\n');

    let sectionHTML = '<h2>' + esc(pageData.section || '') + '</h2>';
    sectionHTML += '<span class="reader-page-num">PDF 第 ' + pageData.page + ' 页';
    if (pageData.page_label) {
        sectionHTML += '（原书第 ' + esc(pageData.page_label) + ' 页）';
    }
    sectionHTML += '</span>';
    document.getElementById('reader-section').innerHTML = sectionHTML;

    document.getElementById('reader-en').innerHTML = formatText(mergedEn);
    document.getElementById('reader-zh').innerHTML = formatText(mergedZh);
    renderVocab(allVocab);
    renderGrammar(allGrammar);

    updatePageNav();
    hideLoading();
}

function formatText(text) {
    if (!text) return '<p class="empty-note">（无内容）</p>';
    // Fix DeepSeek output: "### •" mid-line → separate bullet lines
    text = text.replace(/###\s*•\s*/g, '\n• ');
    // Split into blocks by double newlines
    const blocks = text.split(/\n\n+/);
    let html = '';
    for (let i = 0; i < blocks.length; i++) {
        const trimmed = blocks[i].trim();
        if (!trimmed) continue;

        // Detect markdown table: all non-empty lines start with | and there's a separator line
        if (isMarkdownTable(trimmed)) {
            html += renderMarkdownTable(trimmed);
            continue;
        }

        // Group consecutive » (callout) blocks into a single <p>
        if (isCalloutStart(trimmed)) {
            let groupHTML = '';
            while (i < blocks.length) {
                const b = blocks[i].trim();
                if (!b || !isCalloutStart(b)) break;
                // Split callout blocks by \n for line-by-line rendering.
                // This is essential for marker-prefixed blocks that span
                // multiple lines (e.g. bullet lists within a callout).
                const calloutLines = b.split('\n');
                for (let li = 0; li < calloutLines.length; li++) {
                    const cl = calloutLines[li].trim();
                    if (!cl) continue;
                    let safe, prefix;
                    if (cl.startsWith('»')) {
                        safe = esc(cl.replace(/^»\s*/, ''));
                        prefix = '» ';
                    } else if (cl.startsWith(CALLOUT_CONT_MARKER)) {
                        safe = esc(cl.replace(CALLOUT_CONT_MARKER, ''));
                        prefix = '';
                    } else {
                        safe = esc(cl);
                        prefix = '';
                    }
                    const clickable = safe.replace(/\b([a-zA-Z]+(?:'[a-zA-Z]+)?)\b/g,
                        '<span class="word-clickable" data-word="$1" onclick="lookupWord(this)" title="点击查词">$1</span>');
                    groupHTML += '<span class="reader-callout-line">' + prefix + clickable + '</span>';
                }
                i++;
            }
            i--; // compensate for outer for loop increment
            html += '<p class="callout-group">' + groupHTML + '</p>';
            continue;
        }

        // Split block into individual lines
        const lines = trimmed.split('\n');
        let blockHTML = '';
        lines.forEach(line => {
            const lineTrimmed = line.trim();
            if (!lineTrimmed) return;
            const headingMatch = lineTrimmed.match(/^(#{1,3})\s+(.+)$/);
            // Detect sidebar/callout lines (start with » or invisible marker)
            if (isCalloutStart(lineTrimmed)) {
                let safe, prefix;
                if (lineTrimmed.startsWith('»')) {
                    safe = esc(lineTrimmed.replace(/^»\s*/, ''));
                    prefix = '» ';
                } else {
                    safe = esc(lineTrimmed.replace(CALLOUT_CONT_MARKER, ''));
                    prefix = '';
                }
                const clickable = safe.replace(/\b([a-zA-Z]+(?:'[a-zA-Z]+)?)\b/g,
                    '<span class="word-clickable" data-word="$1" onclick="lookupWord(this)" title="点击查词">$1</span>');
                blockHTML += '<span class="reader-callout-line">' + prefix + clickable + '</span>';
            }
            // Detect heading markers (#, ##, ###)
            else if (headingMatch) {
                const level = headingMatch[1].length; // 1, 2, or 3
                const headingText = esc(headingMatch[2]);
                // # → h2, ## → h3, ### → h4 (h1 is page-level title)
                blockHTML += '<h' + (level + 1) + ' class="reader-heading">' + headingText + '</h' + (level + 1) + '>';
            } else {
                // Body text line — make words clickable
                const safe = esc(lineTrimmed);
                const clickable = safe.replace(/\b([a-zA-Z]+(?:'[a-zA-Z]+)?)\b/g,
                    '<span class="word-clickable" data-word="$1" onclick="lookupWord(this)" title="点击查词">$1</span>');
                blockHTML += '<span class="reader-line">' + clickable + '</span>';
            }
        });
        if (blockHTML) {
            html += '<p>' + blockHTML + '</p>';
        }
    }
    return html || '<p class="empty-note">（无内容）</p>';
}

// isMarkdownTable returns true if the block looks like a markdown pipe table:
// all non-empty lines start and end with |, at least 2 data lines,
// and at least one separator line (|----|----|).
function isMarkdownTable(block) {
    const lines = block.split('\n').map(l => l.trim()).filter(l => l);
    if (lines.length < 2) return false;
    const allPipes = lines.every(l => l.startsWith('|') && l.endsWith('|'));
    if (!allPipes) return false;
    const hasSeparator = lines.some(l => /^\|[\s\-:|]+\|$/.test(l));
    return hasSeparator;
}

// renderMarkdownTable converts a markdown pipe table block to an HTML <table>.
function renderMarkdownTable(block) {
    const lines = block.split('\n').map(l => l.trim()).filter(l => l.startsWith('|'));
    if (lines.length < 2) return '<p class="empty-note">（空表格）</p>';

    let html = '<table class="md-table"><thead>';
    let inHeader = true;

    for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        // Skip separator lines (|----|----|)
        if (/^\|[\s\-:|]+\|$/.test(line)) {
            if (inHeader) {
                html += '</thead><tbody>';
                inHeader = false;
            }
            continue;
        }

        // Split cells: remove leading/trailing |, then split by |
        const rawCells = line.replace(/^\|\s*/, '').replace(/\s*\|$/, '').split('|');
        const tag = inHeader ? 'th' : 'td';

        html += '<tr>';
        rawCells.forEach(cell => {
            const cellText = cell.trim();
            const safe = esc(cellText);
            // Make English words clickable within table cells
            const clickable = safe.replace(/\b([a-zA-Z]+(?:'[a-zA-Z]+)?)\b/g,
                '<span class="word-clickable" data-word="$1" onclick="lookupWord(this)" title="点击查词">$1</span>');
            html += '<' + tag + '>' + clickable + '</' + tag + '>';
        });
        html += '</tr>';
    }

    html += '</tbody></table>';
    return html;
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

function nextPage() {
    if (loading) return;
    fetchPage(currentPage + 1);
}

function prevPage() {
    if (loading || currentPage <= 1) return;
    fetchPage(currentPage - 1);
}

function jumpToPage() {
    if (loading) return;
    const input = document.getElementById('input-jump-page');
    const page = parseInt(input.value);
    if (page > 0) {
        fetchPage(page);
        input.value = '';
    }
}

function updatePageNav() {
    if (!pageData) return;
    document.getElementById('reader-page-display').textContent = '第 ' + currentPage + ' 页';
    document.getElementById('btn-prev-page').disabled = currentPage <= 1;
}

// ============ Page Image ============

function loadPageImage(page) {
    const container = document.getElementById('reader-image-container');
    container.innerHTML =
        '<div class="reader-page-img-wrap">' +
        '<div class="reader-page-img-label">第 ' + page + ' 页</div>' +
        '<img src="' + BASE_PATH + '/api/reader/page-image?book=' + currentBookId + '&page=' + page + '&token=' + encodeURIComponent(getToken()) + '&t=' + Date.now() + '" ' +
        'alt="PDF Page ' + page + '" onerror="this.parentElement.innerHTML=\'<div class=reader-image-placeholder>图片加载失败</div>\'">' +
        '</div>';
}

// ============ Preload Next Page ============

let preloadController = null;  // AbortController for cancelling stale preloads

async function preloadNextPage(page) {
    // Cancel any previous preload that's still in flight
    if (preloadController) {
        preloadController.abort();
    }
    preloadController = new AbortController();

    try {
        // Fire both chunk and image requests in parallel, don't await
        apiFetch('/api/reader/chunk', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({book_id: currentBookId, page: page}),
            signal: preloadController.signal
        }).catch(() => {});  // silently ignore preload errors

        // Also preload the PDF page image
        const img = new Image();
        img.src = BASE_PATH + '/api/reader/page-image?book=' + currentBookId + '&page=' + page + '&token=' + encodeURIComponent(getToken());
    } catch (e) {
        // Silently ignore preload errors
    }
}

// ============ Left Panel Toggle (3-way: PDF / Word / Breakdown) ============

function switchLeftPanel(panel) {
    activePanel = panel;
    const pdfContainer = document.getElementById('reader-image-container');
    const wordPanel = document.getElementById('reader-word-panel');
    const breakdownPanel = document.getElementById('reader-breakdown-panel');
    const returnBtn = document.getElementById('reader-return-word');
    const toggleBtns = document.querySelectorAll('.reader-toggle-btn');

    // Hide all panels
    pdfContainer.style.display = 'none';
    wordPanel.style.display = 'none';
    breakdownPanel.style.display = 'none';
    if (returnBtn) returnBtn.style.display = 'none';

    // Show selected panel
    if (panel === 'pdf') {
        pdfContainer.style.display = '';
        // Show return-to-word button if there's a cached word lookup
        if (lastLookupWord && lastLookupPanelHTML) {
            returnBtn.style.display = 'flex';
        }
    } else if (panel === 'word') {
        wordPanel.style.display = 'flex';
    } else if (panel === 'breakdown') {
        breakdownPanel.style.display = 'flex';
    }

    // Update toggle button active state
    toggleBtns.forEach(b => {
        b.classList.toggle('active', b.getAttribute('data-panel') === panel);
    });
}

// Legacy: called by word lookup when user clicks a word
function showWordPanel() {
    document.getElementById('reader-return-word').style.display = 'none';
    switchLeftPanel('word');
}

// Legacy: called when user closes the word panel to return to PDF
function closeWordPanel(showReturnBtn) {
    // Save current panel state before closing (for return-to-word feature)
    if (showReturnBtn && lastLookupWord) {
        lastLookupPanelHTML = document.getElementById('reader-word-panel-body').innerHTML;
        document.getElementById('return-word-text').textContent = lastLookupWord;
        document.getElementById('reader-return-word').style.display = 'flex';
    }
    switchLeftPanel('pdf');
}

function returnToLastWord() {
    if (!lastLookupWord || !lastLookupPanelHTML) return;
    const panelBody = document.getElementById('reader-word-panel-body');
    panelBody.innerHTML = lastLookupPanelHTML;
    showWordPanel();
}

async function lookupWord(el) {
    const word = el.getAttribute('data-word');
    if (!word) return;

    lastLookupWord = word;
    lastLookupPanelHTML = '';  // will be populated when panel renders
    const panelBody = document.getElementById('reader-word-panel-body');
    showWordPanel();
    panelBody.innerHTML = '<div class="review-loading">正在查询 <strong>' + esc(word) + '</strong>…</div>';

    try {
        // Step 1: Check database first (same logic as index page)
        const qRes = await apiFetch('/api/word/query', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({word: word})
        });
        const qData = await qRes.json();

        if (qData.found && qData.data && qData.data.words && qData.data.words.length > 0) {
            lookupDBWords = qData.data.words;
            lookupAIWords = null;
            if (qData.source === 'global') {
                // 全局缓存 — 和AI翻译展示一致，用户无感知
                lookupMode = 'new';
                panelBody.innerHTML = renderLookupCardNew(qData.data.words);
            } else {
                // 个人数据库 — 已保存
                lookupMode = 'cached';
                panelBody.innerHTML = renderLookupCardCached(qData.data.words);
            }
        } else {
            // Not in database — call AI
            lookupMode = 'new';
            lookupDBWords = null;
            await fetchLookupAI(word, panelBody);
        }
    } catch (err) {
        panelBody.innerHTML = '<div class="error-msg">请求失败: ' + esc(err.message) + '</div>';
    }
}

async function fetchLookupAI(word, modalBody) {
    modalBody.innerHTML = '<div class="review-loading">正在分析 <strong>' + esc(word) + '</strong>…</div>';
    try {
        const res = await apiFetch('/api/chat', {
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
        lookupMode = 'new';
        lookupAIWords = parsed.words;
        modalBody.innerHTML = renderLookupCardNew(parsed.words);
    } catch (err) {
        modalBody.innerHTML = '<div class="error-msg">请求失败: ' + esc(err.message) + '</div>';
    }
}

async function retranslateLookupWord() {
    const panelBody = document.getElementById('reader-word-panel-body');
    if (!lookupDBWords || lookupDBWords.length === 0) return;
    const word = lookupDBWords[0].word;
    await fetchLookupAIForDiff(word, panelBody);
}

// Fetch AI for diff comparison — renders old DB vs new AI side by side
async function fetchLookupAIForDiff(word, modalBody) {
    modalBody.innerHTML = '<div class="review-loading">正在分析 <strong>' + esc(word) + '</strong>…</div>';
    try {
        const res = await apiFetch('/api/chat', {
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
        lookupMode = 'diff';
        lookupAIWords = parsed.words;
        modalBody.innerHTML = renderLookupCardDiff(lookupDBWords, parsed.words);
    } catch (err) {
        modalBody.innerHTML = '<div class="error-msg">请求失败: ' + esc(err.message) + '</div>';
    }
}

// Render diff view — old DB vs new AI side by side (like index page diff)
function renderLookupCardDiff(oldWords, newWords, saveLabel) {
    const label = saveLabel || '💾 保存新版';
    let html = '<div class="action-bar" style="margin-bottom:12px">';
    html += '<span style="color:#7ec8e3;font-weight:600">📋 新旧对比 / Compare</span>';
    html += '<button class="btn-save-all" onclick="saveAllLookupWords()">💾 保存全部新结果</button>';
    html += '</div>';

    const oldMap = new Map(oldWords.map(w => [w.word, w]));
    const newMap = new Map(newWords.map(w => [w.word, w]));

    newWords.forEach((w, idx) => {
        const old = oldMap.get(w.word);

        html += '<div class="diff-compare-box" style="margin-bottom:16px">';

        // Header
        let headerBadges = '';
        if (!old) {
            headerBadges = '<span class="badge-sm">🆕 新发现</span>';
        } else if (!deepEqualLookup(old, w)) {
            headerBadges = '<span class="diff-changed-badge">有变更</span>';
        }
        html += '<div class="diff-compare-box-header">';
        html += '<span class="word-title">' + esc(w.word) + '</span>' + headerBadges;
        html += '</div>';

        // Body — old/new side by side
        html += '<div class="diff-compare-box-body" style="display:flex;gap:6px;flex-wrap:wrap">';

        if (old) {
            html += '<div class="diff-panel old" style="flex:1;min-width:180px">';
            html += '<div class="diff-panel-label">旧版（数据库）</div>';
            html += renderLookupCardContent([old], 'none');
            html += '</div>';
        }

        html += '<div class="diff-panel new" style="flex:1;min-width:180px">';
        html += '<div class="diff-panel-label">新版（AI）</div>';
        html += renderLookupCardContent([w], 'none');
        html += '</div>';

        html += '</div>'; // body

        // Actions
        html += '<div class="diff-compare-actions" style="margin-top:8px;text-align:right">';
        html += '<button class="btn-save" onclick="saveLookupWord(' + idx + ', this)">' + label + '</button>';
        html += '</div>';

        html += '</div>'; // diff-compare-box
    });

    // Old words not in new AI result
    oldWords.forEach(w => {
        if (!newMap.has(w.word)) {
            html += '<div class="diff-compare-box" style="margin-bottom:16px;opacity:0.7">';
            html += '<div class="diff-compare-box-header">';
            html += '<span class="word-title">' + esc(w.word) + '</span>';
            html += '<span class="diff-only-old-badge">旧版独有（新版已移除）</span>';
            html += '</div>';
            html += renderLookupCardContent([w], 'none');
            html += '</div>';
        }
    });

    return html;
}

// Simple deep compare for diff detection (matches index page logic)
function deepEqualLookup(a, b) {
    const ja = JSON.stringify({m: a.meanings, e: a.examples, p: a.pos, d: a.derivation});
    const jb = JSON.stringify({m: b.meanings, e: b.examples, p: b.pos, d: b.derivation});
    return ja === jb;
}

function parseReplyJSON(reply) {
    try { return JSON.parse(reply); } catch (e) {}
    let cleaned = reply;
    const start = cleaned.indexOf('{');
    const end = cleaned.lastIndexOf('}');
    if (start >= 0 && end > start) cleaned = cleaned.slice(start, end + 1);
    try { return JSON.parse(cleaned); } catch (e) { return null; }
}

// Render cached DB data — shows "已保存" notice + "仍要翻译" button (like index page)
function renderLookupCardCached(words) {
    let html = '<div class="action-bar" style="margin-bottom:12px">';
    html += '<span class="cached-notice"><span class="dot"></span> 来自数据库（已保存）</span>';
    html += '<button class="btn-retranslate" onclick="retranslateLookupWord()">🔄 仍要翻译</button>';
    html += '</div>';
    html += renderLookupCardContent(words, 'cached');
    return html;
}

// Render AI result — shows save-all bar + individual save buttons
function renderLookupCardNew(words) {
    let html = '<div class="action-bar" style="margin-bottom:12px">';
    html += '<span style="color:#7ec8e3;font-weight:600">📋 AI 翻译结果</span>';
    html += '<button class="btn-save-all" onclick="saveAllLookupWords()">💾 保存全部</button>';
    html += '</div>';
    html += renderLookupCardContent(words, 'new');
    return html;
}

// Common card rendering shared by cached and new modes
function renderLookupCardContent(words, mode) {
    let html = '';
    words.forEach((w, idx) => {
        html += '<div class="word-card" style="margin-bottom:16px">';
        html += '<div class="word-header">';
        html += '<span class="word-name">' + esc(w.word) + '</span>' +
            '<span class="btn-speak-inline" onclick="event.stopPropagation(); speakWordInline(this)" title="朗读发音（随机音色）">🔊</span>';
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
        if (mode === 'new') {
            html += '<div class="word-actions">';
            html += '<button class="btn-save" onclick="saveLookupWord(' + idx + ', this)">💾 保存此词</button>';
            html += '</div>';
        }
        html += '</div>';
    });
    return html;
}

function getAllVocab() {
    // Build flat vocab array from all chunks
    var result = [];
    if (pageData && pageData.chunks) {
        for (var i = 0; i < pageData.chunks.length; i++) {
            if (pageData.chunks[i].vocab) {
                result = result.concat(pageData.chunks[i].vocab);
            }
        }
    }
    return result;
}

async function saveVocabWord(index, btnEl) {
    var allVocab = getAllVocab();
    if (allVocab.length === 0) return;
    const v = allVocab[index];
    if (!v) return;

    // Step 1: Check if word already exists in DB
    btnEl.disabled = true; btnEl.textContent = '检查中…';
    try {
        const qRes = await apiFetch('/api/word/query', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({word: v.word})
        });
        const qData = await qRes.json();
        if (qData.found && qData.source === 'personal' && qData.data && qData.data.words && qData.data.words.length > 0) {
            // Already exists in personal DB — show diff modal so user can compare before overwriting
            btnEl.textContent = '对比中';
            lookupVocabBtnEl = btnEl;  // remember original button for later update

            // Construct WordEntry from vocab data as "new version"
            const vocabEntry = {
                word: v.word, type: '基础词', pos: v.pos || '', baseWord: null, derivation: null,
                meanings: [{domain: '金融', text: v.definition}],
                examples: v.example ? [{en: v.example, zh: ''}] : []
            };

            // Set state and show diff in panel
            lookupMode = 'diff-vocab';
            lookupDBWords = qData.data.words;
            lookupAIWords = [vocabEntry];
            lastLookupWord = v.word;

            const panelBody = document.getElementById('reader-word-panel-body');
            showWordPanel();
            panelBody.innerHTML = renderLookupCardDiff(qData.data.words, [vocabEntry], '💾 覆盖保存');
            return;
        }
    } catch (err) {
        btnEl.textContent = '失败';
        btnEl.disabled = false;
        return;
    }

    // Step 2: Word doesn't exist — proceed with save
    btnEl.textContent = '保存中…';
    const wordEntry = {
        word: v.word, type: '基础词', pos: v.pos || '', baseWord: null, derivation: null,
        meanings: [{domain: '金融', text: v.definition}],
        examples: v.example ? [{en: v.example, zh: ''}] : []
    };
    try {
        const res = await apiFetch('/api/word/save', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(wordEntry)});
        const data = await res.json();
        btnEl.textContent = data.success ? '已保存' : '失败';
        if (data.success) btnEl.className = 'btn-save-small saved'; else btnEl.disabled = false;
    } catch (err) { btnEl.textContent = '失败'; btnEl.disabled = false; }
}

async function saveLookupWord(index, btnEl) {
    // Read word data from in-memory lookupAIWords (avoids HTML attribute escaping issues)
    let w;
    if (lookupAIWords && lookupAIWords[index]) {
        w = lookupAIWords[index];
    } else {
        btnEl.textContent = '失败';
        return;
    }

    // diff-vocab mode: intentional overwrite, skip DB check
    if (lookupMode === 'diff-vocab') {
        btnEl.disabled = true; btnEl.textContent = '覆盖中…';
        try {
            const res = await apiFetch('/api/word/save', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(w)});
            const data = await res.json();
            btnEl.textContent = data.success ? '已覆盖' : '失败';
            if (data.success) {
                btnEl.className = 'btn-save saved';
                // Also update the original vocab table button
                if (lookupVocabBtnEl) {
                    lookupVocabBtnEl.textContent = '已覆盖';
                    lookupVocabBtnEl.className = 'btn-save-small saved';
                    lookupVocabBtnEl = null;
                }
            } else {
                btnEl.disabled = false;
            }
        } catch (err) { btnEl.textContent = '失败'; btnEl.disabled = false; }
        return;
    }

    // Step 1: Check if word already exists in DB
    btnEl.disabled = true; btnEl.textContent = '检查中…';
    try {
        const qRes = await apiFetch('/api/word/query', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({word: w.word})
        });
        const qData = await qRes.json();
        if (qData.found && qData.source === 'personal') {
            btnEl.textContent = '已存在';
            btnEl.className = 'btn-save saved';
            return;
        }
    } catch (err) {
        btnEl.textContent = '失败';
        btnEl.disabled = false;
        return;
    }

    // Step 2: Word doesn't exist — proceed with save
    btnEl.textContent = '保存中…';
    try {
        const res = await apiFetch('/api/word/save', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(w)});
        const data = await res.json();
        btnEl.textContent = data.success ? '已保存' : '失败';
        if (data.success) btnEl.className = 'btn-save saved'; else btnEl.disabled = false;
    } catch (err) { btnEl.textContent = '失败'; btnEl.disabled = false; }
}

async function saveAllLookupWords() {
    if (!lookupAIWords || lookupAIWords.length === 0) return;
    const allBtns = document.querySelectorAll('#reader-word-panel-body .btn-save, #reader-word-panel-body .btn-save-all');
    allBtns.forEach(b => { b.disabled = true; });
    const saveAllBtn = document.querySelector('#reader-word-panel-body .btn-save-all');
    if (saveAllBtn) { saveAllBtn.textContent = '保存中…'; saveAllBtn.disabled = true; }

    try {
        const res = await apiFetch('/api/word/save-all', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({words: lookupAIWords})
        });
        const data = await res.json();
        if (data.success) {
            if (saveAllBtn) { saveAllBtn.textContent = '✅ 全部已保存 (' + data.count + ')'; }
            // Update all individual save buttons
            document.querySelectorAll('#reader-word-panel-body .btn-save').forEach(b => {
                b.textContent = '已保存';
                b.className = 'btn-save saved';
                b.disabled = true;
            });
            // If diff-vocab mode, also update the original vocab table button
            if (lookupMode === 'diff-vocab' && lookupVocabBtnEl) {
                lookupVocabBtnEl.textContent = '已覆盖';
                lookupVocabBtnEl.className = 'btn-save-small saved';
                lookupVocabBtnEl = null;
            }
        } else {
            if (saveAllBtn) { saveAllBtn.textContent = '失败'; saveAllBtn.disabled = false; }
            allBtns.forEach(b => { b.disabled = false; });
        }
    } catch (err) {
        if (saveAllBtn) { saveAllBtn.textContent = '失败'; saveAllBtn.disabled = false; }
        allBtns.forEach(b => { b.disabled = false; });
    }
}

function speakWordInline(el) {
    const nameEl = el.parentElement.querySelector('.word-name');
    if (!nameEl) return;
    const word = nameEl.textContent.trim();
    if (!word) return;
    const u = createUtterance(word);
    el.classList.add('speaking');
    u.onend = () => el.classList.remove('speaking');
    u.onerror = () => el.classList.remove('speaking');
    safeSpeak(u);
}

// ============ Sentence Breakdown (selection → floating button → API → left panel) ============

let breakdownBtn = null;   // floating breakdown button element
let lastSelection = '';    // last selected text

function initBreakdownUI() {
    // Create floating breakdown button (hidden by default)
    if (breakdownBtn) return;
    breakdownBtn = document.createElement('div');
    breakdownBtn.className = 'breakdown-float-btn';
    breakdownBtn.innerHTML = '🔍 拆解句子';
    breakdownBtn.style.display = 'none';
    breakdownBtn.onclick = function(e) {
        e.stopPropagation();
        breakdownSentence(lastSelection);
        hideBreakdownBtn();
    };
    document.body.appendChild(breakdownBtn);

    // Listen for text selection on the right panel (reader-en / reader-zh blocks)
    const rightPanel = document.querySelector('.reader-right');
    if (rightPanel) {
        rightPanel.addEventListener('mouseup', onRightPanelMouseUp);
    }
    // Also listen on the entire document to detect clicks outside selection
    document.addEventListener('mousedown', onGlobalMouseDown);
}

function onRightPanelMouseUp(e) {
    // Debounce - wait a frame for selection to be available
    setTimeout(() => {
        const sel = window.getSelection();
        const text = (sel ? sel.toString().trim() : '');
        if (!text || text.length < 2) {
            hideBreakdownBtn();
            return;
        }
        // Has spaces → multi-word selection (sentence/phrase)
        if (!text.includes(' ')) {
            // Single word without spaces — don't show breakdown button
            hideBreakdownBtn();
            return;
        }
        // Save selection and position the button near the last selected word
        lastSelection = text;
        const range = sel.getRangeAt(0);
        // Collapse to end to get the last character's position (stays near last word)
        const endRange = range.cloneRange();
        endRange.collapse(false);
        const rect = endRange.getBoundingClientRect();
        // Position button below the selection end, clamped to viewport
        let top = rect.bottom + 8;
        let left = rect.right - 100;
        if (left < 10) left = rect.left;
        if (top > window.innerHeight - 50) top = rect.top - 42;
        if (left + 140 > window.innerWidth) left = window.innerWidth - 150;
        breakdownBtn.style.top = top + 'px';
        breakdownBtn.style.left = left + 'px';
        breakdownBtn.style.display = 'flex';
    }, 10);
}

function onGlobalMouseDown(e) {
    // Hide the floating button if click is outside the button itself
    if (breakdownBtn && e.target !== breakdownBtn && !breakdownBtn.contains(e.target)) {
        hideBreakdownBtn();
    }
}

function hideBreakdownBtn() {
    if (breakdownBtn) {
        breakdownBtn.style.display = 'none';
    }
    lastSelection = '';
}

async function breakdownSentence(text) {
    if (!text) return;
    lastBreakdownSentence = text;
    const panelBody = document.getElementById('reader-breakdown-panel-body');
    switchLeftPanel('breakdown');
    panelBody.innerHTML = '<div class="review-loading">正在拆解句子…<br><small style="color:#94a3b8">AI 正在深度分析语法结构、短语搭配和用词习惯</small></div>';

    try {
        const res = await apiFetch('/api/reader/breakdown-sentence', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({sentence: text})
        });
        const data = await res.json();

        if (data.error) {
            panelBody.innerHTML = '<div class="error-msg">拆解失败: ' + esc(data.error) + '</div>';
            return;
        }

        lastBreakdownHTML = renderBreakdown(data);
        panelBody.innerHTML = lastBreakdownHTML;
    } catch (err) {
        panelBody.innerHTML = '<div class="error-msg">请求失败: ' + esc(err.message) + '</div>';
    }
}

function renderBreakdown(data) {
    let html = '';

    // 原句 + 翻译
    html += '<div class="breakdown-section">';
    html += '<div class="breakdown-sentence-en">' + esc(data.sentence) + '</div>';
    html += '<div class="breakdown-translation">' + esc(data.translation) + '</div>';
    html += '</div>';

    // 语法分析（紧跟翻译，帮助理解句子结构）
    if (data.grammar && data.grammar.length > 0) {
        html += '<div class="breakdown-section">';
        html += '<div class="breakdown-section-title">📐 语法结构 / Grammar</div>';
        data.grammar.forEach(g => {
            html += '<div class="breakdown-grammar-item">' +
                '<div class="breakdown-grammar-point"><strong>' + esc(g.point) + '</strong></div>' +
                '<div class="breakdown-grammar-detail">' + esc(g.detail) + '</div>' +
                '</div>';
        });
        html += '</div>';
    }

    // 逐词分析
    if (data.vocabulary && data.vocabulary.length > 0) {
        html += '<div class="breakdown-section">';
        html += '<div class="breakdown-section-title">📝 逐词解析 / Word by Word</div>';
        html += '<div class="breakdown-vocab-list">';
        data.vocabulary.forEach(v => {
            html += '<span class="breakdown-vocab-item">' +
                '<strong>' + esc(v.word) + '</strong>' +
                ' <span class="breakdown-vocab-pos">' + esc(v.pos) + '</span> ' +
                esc(v.meaning) +
                '</span>';
        });
        html += '</div>';
        html += '</div>';
    }

    // 短语分析
    if (data.phrases && data.phrases.length > 0) {
        html += '<div class="breakdown-section">';
        html += '<div class="breakdown-section-title">🔗 短语与搭配 / Phrases & Collocations</div>';
        data.phrases.forEach(p => {
            html += '<div class="breakdown-phrase-item">' +
                '<div class="breakdown-phrase-text">' + esc(p.phrase) + '</div>' +
                '<div class="breakdown-phrase-meaning">' + esc(p.meaning) + '</div>';
            if (p.note) {
                html += '<div class="breakdown-phrase-note">' + esc(p.note) + '</div>';
            }
            html += '</div>';
        });
        html += '</div>';
    }

    // 使用习惯与文化（最后，扩展阅读）
    if (data.usage_notes && data.usage_notes.trim() && data.usage_notes !== '无' && data.usage_notes !== '无特殊情况') {
        html += '<div class="breakdown-section">';
        html += '<div class="breakdown-section-title">💡 使用习惯与文化 / Usage & Culture</div>';
        html += '<div class="breakdown-usage-notes">' + esc(data.usage_notes) + '</div>';
        html += '</div>';
    }

    return html || '<div class="error-msg">AI 返回了空结果</div>';
}

function closeModal(event) {
    if (event && event.target !== document.getElementById('word-modal')) return;
    document.getElementById('word-modal').style.display = 'none';
}

// ============ UI Helpers ============

function showNoBooks() {
    hideLoading();
    hideError();
    hideEmpty();
    document.getElementById('reader-loading').style.display = 'none';
    document.getElementById('reader-main').style.display = 'none';
    document.getElementById('reader-no-books').style.display = 'block';
    document.getElementById('reader-page-display').textContent = '第 -- 页';
    document.getElementById('reader-subtitle').textContent = '暂无书籍';
}

function showLoading(msg) {
    const el = document.getElementById('reader-loading');
    el.textContent = msg || '正在加载…';
    el.style.display = 'flex';
}
function hideLoading() { document.getElementById('reader-loading').style.display = 'none'; }
function showError(msg) {
    const el = document.getElementById('reader-error');
    el.innerHTML = '<div class="error-msg">' + esc(msg) + '</div>' +
        '<button class="btn-next" onclick="retryCurrentPage()" style="margin-top:16px;">🔄 重试当前页 / Retry</button>';
    el.style.display = 'block';
}
function retryCurrentPage() {
    if (loading) return;
    hideError();
    fetchPage(currentPage);
}
function hideError() { document.getElementById('reader-error').style.display = 'none'; }
function showEmpty(msg) {
    const el = document.getElementById('reader-empty');
    el.querySelector('.all-done-sub').textContent = msg || '此页无文本内容';
    el.style.display = 'block';
}
function hideEmpty() { document.getElementById('reader-empty').style.display = 'none'; }
