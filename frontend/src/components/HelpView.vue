<script setup>
import { ref, onMounted } from 'vue'
import { GetSettings } from '../../wailsjs/go/main/App'

// 当前本地接口监听端口：从设置读取，动态展示
const apiPort = ref(52323)

onMounted(async () => {
  try {
    const s = await GetSettings()
    if (s.apiPort) apiPort.value = s.apiPort
  } catch (e) {
    /* 读取失败保持默认端口展示 */
  }
})
</script>

<template>
  <div class="help">
    <div class="help-head">
      <h2 class="help-title">使用说明</h2>
      <p class="help-sub">M3U8 下载器 —— 快速上手</p>
    </div>

    <div class="help-body">
      <!-- 新建下载 -->
      <section class="hl-card">
        <div class="hl-head">
          <span class="hl-ico">＋</span>
          <h3>新建下载</h3>
        </div>
        <ol class="hl-list">
          <li>粘贴 <b>M3U8 地址</b>（必填，需以 <code>http(s)://</code> 开头）。</li>
          <li>可选填写 <b>文件名</b>，留空则用当前时间戳命名。</li>
          <li>需要时添加 <b>请求头 Headers</b>（如 <code>User-Agent</code>、<code>Referer</code>，部分站点必需）。</li>
          <li>点击「开始下载」，任务进入队列并自动切回下载管理页。</li>
        </ol>
      </section>

      <!-- 下载管理 -->
      <section class="hl-card">
        <div class="hl-head">
          <span class="hl-ico">⤓</span>
          <h3>下载管理</h3>
        </div>
        <ul class="hl-list">
          <li>顶部标签可筛选：下载中 / 已完成 / 失败（默认显示「下载中」）。</li>
          <li><b>排序</b>：下载中按添加时间正序（最早在前），已完成/失败按时间倒序（最新在前）。</li>
          <li><b>安全退出</b>：点击右上角关闭会二次确认，确认后安全停止下载（保留分片）、停止本地接口服务再退出。</li>
          <li>每个任务卡片显示状态、进度、速度、已下载大小与错误原因。</li>
          <li><b>复制链接</b>：卡片中显示下载链接，点击整行即可复制地址（无提示）。</li>
          <li><b>打开文件</b>：下载完成后，卡片上文件路径与「删除」按钮同行显示；点击路径即可用系统默认程序直接打开成品视频。</li>
          <li><b>批量操作</b>：列表顶部「全选/勾选」后——「下载中」可批量取消下载（正在下载的删除分片、未开始的直接取消）；「已完成/失败」可批量清空记录（可勾选是否同时删除本地文件，默认不删除）。</li>
          <li><b>大列表</b>：任务很多时列表采用<b>虚拟滚动</b>，只渲染可见卡片，数百上千条也流畅；顶部搜索框可按文件名或链接筛选。</li>
          <li><b>暂停 / 继续</b>：暂停保留已下载分片，继续时从断点续传。</li>
          <li><b>失败重试（断点续传）</b>：下载中途出错后点击「重试」，程序保留已下载的分片，只补下缺失分片，<b>不会重头开始</b>。</li>
          <li><b>删除</b>：点击删除会<b>同时删除该任务关联的临时分片与已下载成品文件</b>，删除后不可恢复（有二次确认）。</li>
        </ul>
      </section>

      <!-- 视频解析 -->
      <section class="hl-card">
        <div class="hl-head">
          <span class="hl-ico">▶</span>
          <h3>视频解析</h3>
        </div>
        <p class="hl-text">
          输入视频播放地址（如腾讯视频链接），点击「播放」，自动调用解析接口并在线播放。
          解析接口地址与密钥在 <code>frontend/src/components/VideoParser.vue</code> 顶部常量中，可自行替换。
        </p>
      </section>

      <!-- 接口提交下载 -->
      <section class="hl-card">
        <div class="hl-head">
          <span class="hl-ico">⚡</span>
          <h3>接口提交下载（浏览器推送）</h3>
        </div>
        <p class="hl-text">
          应用内置本地 HTTP 服务，可从浏览器或其他程序直接推送下载任务，支持单个与批量。
          监听端口默认 <code>52323</code>，可在「设置 → API 下载端口」中自定义，保存后立即生效。
        </p>
        <p class="hl-code">POST http://127.0.0.1:{{ apiPort }}/api/download</p>
        <p class="hl-text">请求头 <code>Content-Type: application/json</code>（已开启跨域，浏览器可直接调用）。</p>
        <p class="hl-text"><b>单个推送：</b></p>
        <pre class="hl-pre">{ "name": "文件名", "url": "https://example.com/index.m3u8" }</pre>
        <p class="hl-text"><b>批量推送：</b></p>
        <pre class="hl-pre">[
  { "name": "文件1", "url": "https://example.com/a/index.m3u8" },
  { "name": "文件2", "url": "https://example.com/b/index.m3u8" }
]</pre>
        <p class="hl-text">响应：<code>{ "code": 0, "message": "ok", "count": n, "rejected": n }</code>。文件名可留空，留空使用时间戳命名。</p>
      </section>

      <!-- 设置 -->
      <section class="hl-card">
        <div class="hl-head">
          <span class="hl-ico">⚙</span>
          <h3>设置</h3>
        </div>
        <ul class="hl-list">
          <li><b>同时下载的任务数</b>：最多同时有几个任务在下载（默认 3）。</li>
          <li><b>单个任务并发数量</b>：单个任务同时下载的分片数（默认 10）。</li>
          <li><b>文件保存路径</b>：视频成品存放目录。</li>
          <li><b>API 下载端口</b>：接口提交下载的本地监听端口，修改后保存立即生效。</li>
          <li>设置自动持久化到本地 <code>settings.json</code>。</li>
        </ul>
      </section>

      <!-- 多码率 -->
      <section class="hl-card">
        <div class="hl-head">
          <span class="hl-ico">◎</span>
          <h3>多码率与加密</h3>
        </div>
        <ul class="hl-list">
          <li><b>分辨率优先</b>：遇到 Master（多码率）播放列表时，自动选择<b>分辨率最高</b>的子流（无分辨率信息则选码率最高）下载。</li>
          <li><b>AES-128 加密</b>：自动识别并解密加密流。</li>
          <li><b>分片智能过滤</b>：自动剔除与源地址不匹配的无效分片。</li>
        </ul>
      </section>

      <!-- 状态 -->
      <section class="hl-card">
        <div class="hl-head">
          <span class="hl-ico">◔</span>
          <h3>任务状态说明</h3>
        </div>
        <ul class="hl-list">
          <li><b>等待中</b>：已加入队列，等待并发空位。</li>
          <li><b>解析中</b>：正在解析 m3u8 与下载密钥。</li>
          <li><b>下载中</b>：正在并发下载分片。</li>
          <li><b>已完成</b>：全部完成，已合并输出视频。</li>
          <li><b>失败</b>：解析或下载出错，卡片显示错误原因，可重试（断点续传）。</li>
          <li><b>已取消</b>：手动取消，可重试或删除。</li>
        </ul>
      </section>
    </div>
  </div>
</template>

<style scoped>
.help {
  max-width: 820px;
  width: 100%;
  margin: 0 auto;
  padding: 28px 28px 48px;
  box-sizing: border-box;
}
.help-head {
  margin-bottom: 22px;
}
.help-title {
  font-size: 21px;
  font-weight: 600;
}
.help-sub {
  font-size: 13.5px;
  color: var(--text-sub);
  margin-top: 7px;
}
.help-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.hl-card {
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: 14px;
  box-shadow: var(--shadow);
  padding: 18px 22px;
}
.hl-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
}
.hl-ico {
  width: 26px;
  height: 26px;
  border-radius: 8px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 14px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.hl-head h3 {
  font-size: 15px;
  font-weight: 600;
}
.hl-list {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 7px;
  font-size: 13.5px;
  color: var(--text);
  line-height: 1.65;
}
.hl-list li {
  position: relative;
  padding-left: 16px;
}
.hl-list li::before {
  content: '';
  position: absolute;
  left: 2px;
  top: 8px;
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--accent);
}
.hl-text {
  font-size: 13.5px;
  color: var(--text);
  line-height: 1.7;
  margin-bottom: 6px;
}
.hl-code {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 13px;
  color: var(--accent);
  background: var(--accent-soft);
  border-radius: 7px;
  padding: 7px 12px;
  margin: 8px 0;
  display: inline-block;
}
.hl-pre {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 12.5px;
  color: #374151;
  background: #f7f8fa;
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 10px 14px;
  margin: 6px 0 10px;
  overflow-x: auto;
  white-space: pre;
}
code {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 12.5px;
  background: #f0f2f5;
  border-radius: 4px;
  padding: 1px 5px;
  color: #374151;
}
</style>
