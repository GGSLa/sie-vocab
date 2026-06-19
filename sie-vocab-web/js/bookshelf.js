// ── 书架页 JS — bookshelf.js ──

let books = [];
let loading = false;

// ── 页面加载 ──
document.addEventListener('DOMContentLoaded', () => {
    loadBooks();
    setupDragDrop();
});

// ── 加载书架列表 ──
async function loadBooks() {
    const grid = document.getElementById('bookshelf-grid');
    const empty = document.getElementById('bookshelf-empty');
    const error = document.getElementById('bookshelf-error');
    const count = document.getElementById('bookshelf-count');

    grid.style.display = 'block';
    grid.innerHTML = '<div class="review-loading">加载中…</div>';
    empty.style.display = 'none';
    error.style.display = 'none';

    try {
        const resp = await fetch('/api/books');
        const data = await resp.json();

        if (data.books && data.books.length > 0) {
            books = data.books;
            renderBooks();
            count.textContent = `共 ${books.length} 本教材`;
        } else {
            books = [];
            grid.style.display = 'none';
            empty.style.display = 'block';
            count.textContent = '';
        }
    } catch (err) {
        console.error('加载书架失败:', err);
        grid.style.display = 'none';
        error.style.display = 'block';
        error.innerHTML = '<span style="color:#ef4444;">❌ 加载失败，请刷新重试 / Failed to load books</span>';
        count.textContent = '';
    }
}

// ── 渲染书籍卡片 ──
function renderBooks() {
    const grid = document.getElementById('bookshelf-grid');
    grid.style.display = 'grid';

    let html = '';
    for (const book of books) {
        const pageInfo = book.page_count > 0 ? `${book.page_count} 页` : '页数未知';
        const ocrBadge = `<span class="book-badge">OCR: ${book.ocr_lang}</span>`;
        const created = book.created_at ? book.created_at.substring(0, 10) : '';

        html += `
        <div class="book-card">
            <div class="book-card-header">
                <span class="book-icon">📄</span>
                <span class="book-title">${escHtml(book.title)}</span>
            </div>
            <div class="book-card-body">
                <div class="book-meta">
                    ${book.author ? `<span>✍ ${escHtml(book.author)}</span>` : ''}
                    <span>📖 ${pageInfo}</span>
                    ${ocrBadge}
                </div>
                ${book.description ? `<div class="book-desc">${escHtml(book.description)}</div>` : ''}
                ${created ? `<div class="book-date">📅 添加于 ${created}</div>` : ''}
            </div>
            <div class="book-card-actions">
                <button class="btn-book-read" onclick="openBook(${book.id})">📖 阅读</button>
                <button class="btn-book-delete" onclick="deleteBook(${book.id}, '${escHtml(book.title)}')">🗑 删除</button>
            </div>
        </div>`;
    }

    grid.innerHTML = html;
}

// ── 打开书籍 ──
function openBook(id) {
    window.location.href = `/reader.html?book=${id}`;
}

// ── 删除书籍 ──
async function deleteBook(id, title) {
    if (!confirm(`确定删除「${title}」吗？\n\n此操作将同时删除：\n• PDF 文件\n• AI 分析缓存\n• 阅读进度\n\n此操作不可撤销。`)) {
        return;
    }

    try {
        const resp = await fetch(`/api/books?id=${id}`, { method: 'DELETE' });
        const data = await resp.json();
        if (data.success) {
            loadBooks();
        } else {
            alert('删除失败，请重试');
        }
    } catch (err) {
        console.error('删除书籍失败:', err);
        alert('删除失败: ' + err.message);
    }
}

// ── 上传模态框 ──
function openUploadModal() {
    document.getElementById('upload-modal').style.display = 'flex';
    document.getElementById('upload-title').focus();
}

function closeUploadModal(event) {
    if (event && event.target !== document.getElementById('upload-modal')) return;
    document.getElementById('upload-modal').style.display = 'none';
    document.getElementById('upload-form').reset();
    document.getElementById('dropzone-text').textContent = '📁 点击选择文件 或拖拽到此处 / Click or drag PDF here';
    document.getElementById('btn-upload-submit').disabled = false;
    document.getElementById('btn-upload-submit').textContent = '📤 上传';
}

// ── 上传书籍 ──
async function uploadBook(event) {
    event.preventDefault();

    const fileInput = document.getElementById('upload-file');
    const file = fileInput.files[0];
    if (!file) {
        alert('请选择 PDF 文件');
        return;
    }
    if (!file.name.toLowerCase().endsWith('.pdf')) {
        alert('仅支持 PDF 文件');
        return;
    }

    const btn = document.getElementById('btn-upload-submit');
    btn.disabled = true;
    btn.textContent = '⏳ 上传中…';

    const formData = new FormData();
    formData.append('file', file);
    formData.append('title', document.getElementById('upload-title').value.trim() || file.name.replace(/\.pdf$/i, ''));
    formData.append('author', document.getElementById('upload-author').value.trim());
    formData.append('description', document.getElementById('upload-desc').value.trim());
    formData.append('ocr_lang', document.getElementById('upload-ocr').value);

    try {
        const resp = await fetch('/api/books', {
            method: 'POST',
            body: formData,
        });

        if (resp.ok) {
            const book = await resp.json();
            console.log('上传成功:', book);
            closeUploadModal();
            loadBooks();
        } else {
            const err = await resp.json();
            alert('上传失败: ' + (err.error || '未知错误'));
        }
    } catch (err) {
        console.error('上传失败:', err);
        alert('上传失败: ' + err.message);
    }

    btn.disabled = false;
    btn.textContent = '📤 上传';
}

// ── 拖拽上传 ──
function setupDragDrop() {
    const dropzone = document.getElementById('upload-dropzone');
    if (!dropzone) return;

    ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
        dropzone.addEventListener(eventName, e => {
            e.preventDefault();
            e.stopPropagation();
        });
    });

    ['dragenter', 'dragover'].forEach(eventName => {
        dropzone.addEventListener(eventName, () => {
            dropzone.classList.add('upload-dropzone-active');
        });
    });

    ['dragleave', 'drop'].forEach(eventName => {
        dropzone.addEventListener(eventName, () => {
            dropzone.classList.remove('upload-dropzone-active');
        });
    });

    dropzone.addEventListener('drop', e => {
        const files = e.dataTransfer.files;
        if (files.length > 0) {
            document.getElementById('upload-file').files = files;
            updateDropzoneLabel();
        }
    });
}

function updateDropzoneLabel() {
    const fileInput = document.getElementById('upload-file');
    const dropzoneText = document.getElementById('dropzone-text');
    if (fileInput.files.length > 0) {
        const f = fileInput.files[0];
        const sizeMB = (f.size / (1024 * 1024)).toFixed(1);
        dropzoneText.textContent = `✅ ${f.name} (${sizeMB} MB)`;
    } else {
        dropzoneText.textContent = '📁 点击选择文件 或拖拽到此处 / Click or drag PDF here';
    }
}

// ── HTML 转义 ──
function escHtml(s) {
    if (!s) return '';
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#x27;');
}
