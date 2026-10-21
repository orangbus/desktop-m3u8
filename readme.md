# M3U8 下载器（desk-m3u8）

基于 **Wails (Go + Vue 3)** 构建的桌面版 M3U8 视频下载器，极简卡片式界面，开箱即用。

支持解析、并发下载分片、AES-128 解密、自动合并成视频文件；内置浏览器推送下载接口与在线视频解析功能。

![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)
![Wails](https://img.shields.io/badge/Wails-2.10-2f6bff?logo=wails)
![Vue](https://img.shields.io/badge/Vue-3-4FC08D?logo=vue.js)
![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey)

---

## ✨ 功能特性

- **M3U8 视频下载**：输入链接 → 自动解析 → 并发下载分片 → 合并为视频文件
- **多任务并发**：可自定义「同时下载的任务数」与「单个任务分片并发数」
- **分辨率优先**：遇到 Master（多码率）播放列表，**自动选择分辨率最高的子流**（无分辨率信息则选码率最高的）下载
- **失败重试 / 断点续传**：下载出错后点击「重试」，**保留已下载分片，只补下缺失分片，不重头开始**
- **删除关联文件**：删除任务时**同时清理临时分片与已下载的成品文件**
- **AES-128 解密**：自动识别并解密加密流
- **自定义请求头**：支持为每个任务设置 `User-Agent`、`Referer` 等 Header（部分站点必需）
- **浏览器推送接口**：内置本地 HTTP 服务，支持从浏览器 **POST 单个/批量** 推送下载任务，**端口可在设置中自定义**
- **在线视频解析**：输入播放地址，自动调用解析接口并 iframe 播放
- **实时进度**：任务进度条、下载速度、已下载大小实时刷新
- **一键复制链接**：任务卡片中直接显示下载链接，点击整行即可复制地址
- **一键打开文件**：下载完成后卡片上本地路径与「删除」同行显示，点击路径直接用系统默认程序打开成品视频
- **虚拟滚动 + 搜索**：数百上千条任务也只渲染可见卡片、流畅滚动，可按文件名/链接关键字筛选
- **状态筛选**：下载中 / 已完成 / 失败 一键筛选（默认显示下载中）
- **任务记录持久化**：下载记录自动保存到本地，重启软件后历史任务不丢失；上次未完成的任务可直接「重试」断点续传
- **应用内使用说明**：左侧菜单「使用说明」提供完整操作指引
- **设置持久化**：并发数、保存路径、API 端口、接口地址/授权密钥自动保存到本地
- **默认下载目录**：首次启动自动将下载目录设为软件所在目录（可随时在设置中修改）
- **GitHub Actions 三端自动发布**：提交 `v*` 标签自动打包 Windows / macOS / Linux 并发布

---

## 🖥️ 界面预览

- 左侧深色导航栏：**新建下载 / 下载管理 / 视频解析 / 使用说明** + 底部「设置」
- 右侧主区：顶部栏为无边框窗口拖拽区（最小化 / 最大化 / 关闭）
- 下载管理：筛选标签 + 紧凑任务卡片 + 底部统计栏（总数 / 已完成 / 失败 / 总速度）

（可自行运行后在「下载管理」页添加截图替换此说明。）

---

## 🛠️ 技术栈

| 层 | 技术 |
|---|---|
| 桌面框架 | [Wails v2.10](https://wails.io/)（Go 后端 + WebView2） |
| 后端 | Go 1.25 |
| 前端 | Vue 3 + Vite 7（极简卡片式 UI） |
| 打包发布 | GitHub Actions（release.yml，tag 触发三端构建） |

---

## 🚀 快速开始

### 直接运行

双击打包好的可执行文件即可（Windows 需已安装 WebView2 运行时，Win10/11 一般已内置）：

```
build\bin\desk-m3u8.exe
```

### 开发模式（热更新）

```powershell
wails dev
```

`wails dev` 会自动：启动 Vite 前端开发服务器（热更新）→ 编译启动 Go 后端 → 每次改动自动重建绑定与页面。

> 前置要求：Go ≥ 1.25、Node.js ≥ 20、Wails CLI（`go install github.com/wailsapp/wails/v2/cmd/wails@latest`）。

### 打包构建

```powershell
wails build        # 产物：build\bin\desk-m3u8.exe
wails build --clean  # 强制重新构建前端
# 跨平台：wails build -platform windows/amd64 / darwin/arm64 / linux/amd64
```

### GitHub Actions 自动发布

仓库内置 `.github/workflows/release.yml`：提交 `v*` 版本标签时自动打包三端并上传到 GitHub Releases。

```bash
git add .
git commit -m "release: v1.0.0"
git tag v1.0.0
git push origin v1.0.0 --tags
```

| 产物 | 平台 |
|---|---|
| `desk-m3u8-windows-amd64.zip` | Windows x64（exe） |
| `desk-m3u8-darwin-arm64.zip` / `-darwin-amd64.zip` | macOS Apple Silicon / Intel（.app） |
| `desk-m3u8-linux-amd64.tar.gz` | Linux x64 |

---

## 📖 使用说明

### 新建下载

填写 **M3U8 地址**（必填）、**文件名**（可选，留空用时间戳命名）、**请求头 Headers**（可选），点击「开始下载」。

### 下载管理

每个任务卡片显示状态、进度、速度、已下载大小与错误原因；支持暂停 / 继续 / 取消 / 重试 / 删除。

- **暂停 / 继续**：保留已下载分片，从断点续传。
- **失败重试（断点续传）**：保留已下载分片，只补下缺失分片，不重头开始。
- **删除**：同时清理该任务关联的临时分片与成品文件（有二次确认）。

### 设置

- 同时下载的任务数（默认 3）
- 单个任务分片并发数（默认 10）
- 文件保存路径
- **API 下载端口**（默认 `52323`，保存后立即生效）
- 接口地址 / 授权密钥（点击左侧顶部账户信息配置）

### 浏览器推送下载接口

应用内置本地 HTTP 服务，可从浏览器或其他程序直接推送下载任务，支持单个与批量。

```
POST http://127.0.0.1:<端口>/api/download
```

请求头：`Content-Type: application/json`（已开启跨域 CORS）。

**单个推送：**

```json
{ "name": "文件名", "url": "https://example.com/index.m3u8" }
```

**批量推送：**

```json
[
  { "name": "文件1", "url": "https://example.com/a/index.m3u8" },
  { "name": "文件2", "url": "https://example.com/b/index.m3u8" }
]
```

**响应：**

```json
{ "code": 0, "message": "ok", "count": 2, "rejected": 0 }
```

**浏览器调用示例：**

```js
fetch('http://127.0.0.1:52323/api/download', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ name: '我的视频', url: 'https://example.com/index.m3u8' })
})
```

---

## ⚙️ 配置

设置自动持久化到本地 `settings.json`（路径由 `os.UserConfigDir()` 决定，如 `%APPDATA%\desk-m3u8\settings.json`），字段：

| 字段 | 说明 |
|---|---|
| `maxConcurrentTasks` | 同时下载的任务数 |
| `concurrencyPerTask` | 单个任务分片并发数 |
| `savePath` | 保存路径 |
| `apiPort` | 本地接口下载端口（默认 52323） |
| `apiBaseUrl` | 接口地址 |
| `apiKey` | 授权密钥 |

---

## 🧩 项目结构

```
desk-m3u8/
├── main.go              # 程序入口与 Wails 窗口配置
├── app.go               # Wails 绑定方法 + 本地 HTTP 推送接口（端口可配置）
├── core/                # 下载核心逻辑
│   ├── http.go          # HTTP 客户端（支持自定义请求头）
│   ├── parse.go         # M3U8 解析、多码率最高分辨率选择、分片过滤
│   ├── download.go      # 分片下载、AES 解密、合并
│   ├── manager.go       # 任务调度（并发控制、断点续传、重试/删除）
│   ├── task.go          # 任务与状态定义
│   └── settings.go      # 设置持久化（含 API 端口）
├── frontend/            # Vue3 前端
│   ├── src/App.vue      # 主界面（左侧导航 + 右侧主区）
│   └── src/components/  # NewDownloadForm / VideoParser / SettingsModal / AccountModal / ConfirmDialog / HelpView
├── .github/workflows/   # GitHub Actions（release.yml：tag 触发三端打包发布）
└── build/bin/           # 打包产物（desk-m3u8.exe）
```

---

## ⚠️ 声明

本工具仅供学习与技术交流。请遵守目标网站的服务条款与相关法律法规，**仅下载你有权下载的内容**（如自有视频、公开授权素材等）。请勿用于侵犯他人版权的用途。

---

## 📄 许可证

[MIT](LICENSE)
