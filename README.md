# SIE Vocab — DeepSeek Chat 单词记忆工具

SIE 考试单词记忆辅助工具。前端单词翻译 + 复习 + 书架 + 教材阅读四页面，后端 Go 分层架构调用 DeepSeek API，MySQL 持久化。

## 功能特性

- **单词翻译**：输入单词 → 查本地缓存 → 无缓存则调 DeepSeek AI 翻译 → 展示释义/例句
- **间隔重复复习**：艾宾浩斯算法，每日上限 30 词，凌晨 4 点为日期分割线，三种模式（每日/自由/总览）
- **书架管理**：多 PDF 上传/删除/选书，卡片网格展示
- **教材阅读**：多书逐页阅读，左侧 PDF 原页 + 可折叠目录侧边栏，右侧 AI 翻译/词汇/语法分析
- **智能 PDF 解析**：标题自动检测（字体大小+粗体），硬换行智能合并（两遍），连字符断词处理
- **TTS 发音**：Web Speech API 美式朗读（en-US），零网络调用
- **API 限流**：令牌桶限流器，可配置 RPM 与并发数

## 技术栈

| 层 | 技术 |
|---|---|
| 前端 | 纯 HTML/CSS/JS（无框架），Google Fonts Inter |
| 后端 | Go 标准库 `net/http`，三层架构 (service → logic → repo) |
| ORM | GORM (`gorm.io/gorm` + `gorm.io/driver/mysql`) |
| 数据库 | MySQL 8.0 |
| AI | DeepSeek API |
| PDF | pdftotext / pdftohtml / pdftoppm (Poppler) |

## 项目结构

```
sie-vocab/
├── sie-vocab-web/               # 前端 — 纯 HTML/CSS/JS
│   ├── index.html               # 首页：单词翻译
│   ├── review.html              # 复习页：间隔重复抽词 + 三模式
│   ├── bookshelf.html           # 书架页：PDF 上传/删除/选书
│   ├── reader.html              # 阅读页：逐页阅读 + AI 分析
│   ├── css/style.css            # 全部样式（深色 premium 主题）
│   └── js/
│       ├── app.js               # 首页逻辑
│       ├── review.js            # 复习逻辑
│       ├── bookshelf.js         # 书架逻辑
│       └── reader.js            # 阅读逻辑
├── sie-vocab-server/            # 后端 — Go 分层架构
│   ├── main.go                  # 入口
│   ├── model/                   # 数据结构 + 常量（8 文件）
│   ├── client/                  # DeepSeek API + 令牌桶限流
│   ├── repo/                    # 数据访问层（10 文件，GORM）
│   ├── logic/                   # 业务编排层（14 文件）
│   ├── pdf/pdf.go               # PDF 文本/大纲提取
│   └── service/service.go       # HTTP 适配层（18 个 HandleFunc）
└── sie-vocab-bin/               # 本地运行文件
    ├── config.example.json      # 配置模板
    └── ...
```

## 数据库

MySQL 8.0，库名 `sie_vocab`，共 9 张表：

| 表 | 用途 |
|---|---|
| `books` | 书籍主表 |
| `reader_progress` | 阅读进度（DB 持久化） |
| `words` | 单词主表（基础词/衍生词） |
| `meanings` | 释义（金融/日常双 domain） |
| `examples` | 例句 |
| `review_logs` | 每日复习记录（驱动间隔重复） |
| `free_review_logs` | 自由复习记录 |
| `reader_cache` | AI 分析缓存（复合主键 book_id+page） |
| `daily_stats` | 每日复习快照 |

## API 路由（端口 8080）

### 翻译
| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/chat` | POST | AI 翻译 |
| `/api/word/query` | POST | 查单词族 |
| `/api/word/save` | POST | 保存单词 |
| `/api/word/save-all` | POST | 批量保存 |

### 复习
| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/review/random` | POST | 每日模式抽词 |
| `/api/review/record` | POST | 每日模式记录 |
| `/api/review/free/random` | POST | 自由模式抽词 |
| `/api/review/free/record` | POST | 自由模式记录 |
| `/api/review/overview` | POST | 月度总览 |

### 书架
| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/books` | GET | 列表 / 单本详情 |
| `/api/books` | POST | 上传 PDF（multipart） |
| `/api/books?id=N` | DELETE | 删除书籍 |

### 阅读
| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/reader/chunk` | POST | 取单页 AI 分析 |
| `/api/reader/progress` | GET/POST | 阅读进度 |
| `/api/reader/toc` | GET | 目录大纲 |
| `/api/reader/page-image` | GET | PDF 页图片渲染 |

## 艾宾浩斯间隔重复

| 复习次数 | 间隔 |
|---|---|
| 1 | 1 天 |
| 2 | 2 天 |
| 3 | 4 天 |
| 4 | 7 天 |
| 5 | 15 天 |
| 6+ | 30 天 |

核心规则：
- 每日上限 30 词
- 一日一词族一次（同一词族每天最多复习一个）
- 过期处理：`next_review_date < 今天`，间隔回到 1 天，`review_count` 不重置
- 自由模式不影响间隔重复计数

## 运行

```bash
# 编译
cd sie-vocab-server && go build -o ../sie-vocab-bin/server .

# 启动（server 和 config.json 须在同一目录）
cd sie-vocab-bin && nohup ./server > /tmp/go-server.log 2>&1 &

# 验证
curl -s http://localhost:8080/ | head -5
```

## 配置

参考 `sie-vocab-bin/config.example.json`，主要字段：

```json
{
  "mysql": {
    "host": "localhost:3306",
    "user": "...",
    "password": "...",
    "database": "sie_vocab"
  },
  "deepseek_api_key": "...",
  "deepseek_rpm": 10,
  "deepseek_max_concurrent": 3,
  "upload_dir": "/var/sie-vocab/pdfs",
  "serve_static": true
}
```

## 已知限制

- 单用户模式，无认证系统
- 前端无构建工具，纯手写 HTML/CSS/JS
- 过期判断使用 4AM 分割线，可能与前端时区有差异
- AI 分析首次请求需等待 DeepSeek 响应（约 30s-2min），缓存后毫秒返回
- PDF 硬换行合并为启发式算法，极少数边缘情况可能合并不足或过度

## 设计

深色 premium 主题，蕾姆蓝配色（主色 `#7ec8e3`），玻璃拟态卡片，Google Fonts Inter 字体。
