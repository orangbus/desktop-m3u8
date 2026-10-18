<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { GetTasks, PauseTask, ResumeTask, CancelTask, RemoveTask, RetryTask } from '../wailsjs/go/main/App'
import { EventsOn, WindowMinimise, WindowToggleMaximise, Quit } from '../wailsjs/runtime/runtime'
import NewDownloadForm from './components/NewDownloadForm.vue'
import VideoParser from './components/VideoParser.vue'
import SettingsModal from './components/SettingsModal.vue'
import AccountModal from './components/AccountModal.vue'
import ConfirmDialog from './components/ConfirmDialog.vue'

const tasks = ref([])
const filter = ref('all')
const showSettings = ref(false)
const showAccount = ref(false)
const hasLoaded = ref(false)

const removedIds = new Set()

const confirmState = ref({ show: false, id: null, title: '', message: '' })

// 左侧菜单栏顶部账户信息（姓名/邮箱）：点击弹出接口配置
const account = {
  name: '示例用户',
  email: 'user@example.com',
}

// 左侧导航菜单：后续在此追加新功能项
const menu = [
  { key: 'new', label: '新建下载', icon: '＋' },
  { key: 'download', label: '下载管理', icon: '⤓' },
  { key: 'player', label: '视频解析', icon: '▶' },
]
const currentNav = ref('download')
const currentTitle = computed(() => {
  const m = menu.find((x) => x.key === currentNav.value)
  return m ? m.label : ''
})
function onNav(m) {
  currentNav.value = m.key
}

function winMin() {
  WindowMinimise()
}
function winMax() {
  WindowToggleMaximise()
}
function winClose() {
  Quit()
}

const filters = [
  { key: 'all', label: '全部' },
  { key: 'downloading', label: '下载中' },
  { key: 'done', label: '已完成' },
  { key: 'failed', label: '失败' },
]

const filteredTasks = computed(() => {
  switch (filter.value) {
    case 'downloading':
      return tasks.value.filter((t) =>
        ['waiting', 'parsing', 'downloading', 'paused'].includes(t.status)
      )
    case 'done':
      return tasks.value.filter((t) => t.status === 'done')
    case 'failed':
      return tasks.value.filter((t) => ['failed', 'cancelled'].includes(t.status))
    default:
      return tasks.value
  }
})

const stats = computed(() => {
  const total = tasks.value.length
  const done = tasks.value.filter((t) => t.status === 'done').length
  const failed = tasks.value.filter((t) => t.status === 'failed').length
  let speed = 0
  for (const t of tasks.value) {
    if (t.status === 'downloading') speed += t.speed || 0
  }
  return { total, done, failed, speed }
})

const statusMeta = {
  waiting: { label: '等待中', cls: 'wait' },
  parsing: { label: '解析中', cls: 'download' },
  downloading: { label: '下载中', cls: 'download' },
  paused: { label: '已暂停', cls: 'wait' },
  done: { label: '已完成', cls: 'done' },
  failed: { label: '失败', cls: 'failed' },
  cancelled: { label: '已取消', cls: 'wait' },
}

let unsubscribe = null

function upsertTask(t) {
  if (removedIds.has(t.id)) return // 已删除的任务不再加回
  const i = tasks.value.findIndex((x) => x.id === t.id)
  if (i >= 0) tasks.value[i] = { ...tasks.value[i], ...t }
  else tasks.value.push({ ...t })
}

async function loadTasks() {
  const list = await GetTasks()
  tasks.value = list
  hasLoaded.value = true
}

function formatSpeed(bps) {
  if (!bps) return '0 B/s'
  if (bps >= 1024 * 1024) return (bps / 1024 / 1024).toFixed(1) + ' MB/s'
  if (bps >= 1024) return (bps / 1024).toFixed(0) + ' KB/s'
  return bps.toFixed(0) + ' B/s'
}

function formatBytes(n) {
  if (!n) return '0 B'
  if (n >= 1024 * 1024 * 1024) return (n / 1024 / 1024 / 1024).toFixed(2) + ' GB'
  if (n >= 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB'
  if (n >= 1024) return (n / 1024).toFixed(0) + ' KB'
  return n + ' B'
}

function progressOf(t) {
  if (t.status === 'done') return 100
  if (t.segments > 0) {
    return Math.min(100, Math.round(((t.doneSegs || 0) / t.segments) * 100))
  }
  return t.progress || 0
}

function isRunning(t) {
  return ['waiting', 'parsing', 'downloading'].includes(t.status)
}

function pause(id) {
  PauseTask(id)
}
function resume(id) {
  ResumeTask(id)
}
function cancel(id) {
  CancelTask(id)
}
function retry(id) {
  RetryTask(id)
}
function remove(id) {
  confirmState.value = {
    show: true,
    id,
    title: '删除任务',
    message: '确定删除该任务吗？将同时删除已下载的文件，删除后不可恢复。',
  }
}
function cancelRemove() {
  confirmState.value.show = false
}
function confirmRemove() {
  const id = confirmState.value.id
  if (id) {
    removedIds.add(id)
    RemoveTask(id)
    tasks.value = tasks.value.filter((t) => t.id !== id)
  }
  confirmState.value.show = false
}

// 新建下载成功后，刷新任务列表并切回「下载管理」
function onAdded() {
  loadTasks()
  currentNav.value = 'download'
}

onMounted(async () => {
  await loadTasks()
  unsubscribe = EventsOn('task:update', (t) => upsertTask(t))
})

onUnmounted(() => {
  if (unsubscribe) unsubscribe()
})
</script>

<template>
  <div class="app-shell">
    <!-- 左侧导航栏 -->
    <aside class="sidebar">
      <div class="sb-account" title="点击配置接口地址与授权密钥" @click="showAccount = true">
        <span class="acc-avatar">{{ (account.name || '用')[0] }}</span>
        <div class="acc-meta">
          <span class="acc-name">{{ account.name }}</span>
          <span class="acc-email">{{ account.email }}</span>
        </div>
        <span class="acc-arrow">›</span>
      </div>

      <div class="sb-menu">
        <div class="sb-group-label">功能</div>
        <button
          v-for="m in menu"
          :key="m.key"
          class="sb-item"
          :class="{ active: currentNav === m.key }"
          @click="onNav(m)"
        >
          <span class="sb-ico">{{ m.icon }}</span>
          <span class="sb-label">{{ m.label }}</span>
        </button>
      </div>

      <div class="sb-footer">
        <button class="sb-item settings" @click="showSettings = true">
          <span class="sb-ico">⚙</span>
          <span class="sb-label">设置</span>
        </button>
      </div>
    </aside>

    <!-- 右侧主区 -->
    <div class="main">
      <header class="main-topbar">
        <div class="main-title">{{ currentTitle }}</div>
        <div class="tb-actions">
          <button class="win-btn" title="最小化" @click="winMin">─</button>
          <button class="win-btn" title="最大化/还原" @click="winMax">□</button>
          <button class="win-btn close" title="关闭" @click="winClose">✕</button>
        </div>
      </header>

      <!-- 新建下载页（内嵌，非弹窗） -->
      <div v-if="currentNav === 'new'" class="new-page">
        <NewDownloadForm @added="onAdded" />
      </div>

      <!-- 视频解析页 -->
      <div v-else-if="currentNav === 'player'" class="player-page">
        <VideoParser />
      </div>

      <!-- 下载管理页 -->
      <template v-else>
        <nav class="filters">
          <button
            v-for="f in filters"
            :key="f.key"
            class="filter-tab"
            :class="{ active: filter === f.key }"
            @click="filter = f.key"
          >
            {{ f.label }}
          </button>
        </nav>

        <main class="task-area">
          <div v-if="!hasLoaded" class="empty">加载中…</div>
          <div v-else-if="filteredTasks.length === 0" class="empty">
            <div class="empty-icon">⤓</div>
            <p>暂无{{ filter === 'all' ? '' : '该状态下的' }}任务</p>
            <p class="empty-sub">左侧「新建下载」添加任务</p>
          </div>

          <div v-else class="task-list">
            <div v-for="t in filteredTasks" :key="t.id" class="task-card">
              <div class="card-top">
                <div class="card-title-wrap">
                  <span class="card-title" :title="t.url">{{ t.filename || t.url }}</span>
                  <span class="status-badge" :class="statusMeta[t.status]?.cls">
                    {{ statusMeta[t.status]?.label || t.status }}
                  </span>
                </div>
                <span class="card-meta-right">
                  <span v-if="t.status === 'downloading'" class="speed">{{ formatSpeed(t.speed) }}</span>
                  <span class="size">{{ formatBytes(t.totalBytes) }}</span>
                </span>
              </div>

              <div class="card-mid">
                <div class="progress-wrap">
                  <div class="progress-track">
                    <div class="progress-fill" :style="{ width: progressOf(t) + '%' }"></div>
                  </div>
                  <span class="progress-text">{{ progressOf(t) }}%</span>
                </div>

                <div class="card-actions">
                  <template v-if="isRunning(t)">
                    <button class="action-btn pause" @click="pause(t.id)">暂停</button>
                    <button class="action-btn cancel" @click="cancel(t.id)">取消</button>
                  </template>
                  <template v-else-if="t.status === 'paused'">
                    <button class="action-btn resume" @click="resume(t.id)">继续</button>
                    <button class="action-btn cancel" @click="cancel(t.id)">取消</button>
                  </template>
                  <template v-else>
                    <button
                      v-if="['failed', 'cancelled'].includes(t.status)"
                      class="action-btn retry"
                      @click="retry(t.id)"
                    >
                      重试
                    </button>
                    <button class="action-btn del" @click="remove(t.id)">删除</button>
                  </template>
                </div>
              </div>

              <div
                v-if="t.error || (t.status === 'done' && t.outputPath)"
                class="card-sub"
                :title="t.error || t.outputPath"
              >
                <span v-if="t.error" class="card-error">{{ t.error }}</span>
                <span v-else-if="t.status === 'done' && t.outputPath" class="card-output">
                  📁 {{ t.outputPath }}
                </span>
              </div>
            </div>
          </div>
        </main>

        <footer class="statbar">
          <span>共 {{ stats.total }} 个任务</span>
          <span class="dot">·</span>
          <span>已完成 {{ stats.done }}</span>
          <span class="dot">·</span>
          <span class="stat-fail">失败 {{ stats.failed }}</span>
          <span class="spacer"></span>
          <span v-if="stats.speed > 0" class="stat-speed">总速度 {{ formatSpeed(stats.speed) }}</span>
        </footer>
      </template>
    </div>

    <SettingsModal :show="showSettings" @close="showSettings = false" @saved="loadTasks" />
    <AccountModal :show="showAccount" @close="showAccount = false" @saved="loadTasks" />
    <ConfirmDialog
      :show="confirmState.show"
      title="删除任务"
      :message="confirmState.message"
      confirm-text="删除"
      :danger="true"
      @confirm="confirmRemove"
      @cancel="cancelRemove"
    />
  </div>
</template>

<style scoped>
.app-shell {
  display: flex;
  height: 100vh;
  background: var(--bg);
}

/* ===== 左侧导航栏 ===== */
.sidebar {
  width: 212px;
  flex-shrink: 0;
  background: #1c2430;
  color: #c7d0dc;
  display: flex;
  flex-direction: column;
  user-select: none;
}
.sb-menu {
  padding: 14px 10px 8px;
  display: flex;
  flex-direction: column;
  gap: 3px;
  flex: 1;
  overflow-y: auto;
  --wails-draggable: no-drag;
}
.sb-group-label {
  font-size: 11px;
  color: #67738a;
  padding: 8px 8px 5px;
  letter-spacing: 1px;
}
.sb-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 10px;
  border-radius: 8px;
  color: #aab5c5;
  font-size: 13.5px;
  text-align: left;
  transition: background 0.12s, color 0.12s;
}
.sb-item:hover {
  background: rgba(255, 255, 255, 0.06);
  color: #e6ecf3;
}
.sb-item.active {
  background: var(--accent);
  color: #fff;
}
.sb-ico {
  width: 18px;
  text-align: center;
  font-size: 15px;
}
.sb-label {
  flex: 1;
}
.sb-footer {
  padding: 10px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  --wails-draggable: no-drag;
}
.sb-footer .sb-item.settings {
  width: 100%;
  box-sizing: border-box;
}

/* ===== 右侧主区 ===== */
.main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg);
}
.main-topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 46px;
  padding: 0 12px 0 18px;
  background: #fff;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
  --wails-draggable: drag; /* 无边框窗口拖拽区：右侧顶部 */
  user-select: none;
  cursor: default;
}
.main-title {
  font-size: 15px;
  font-weight: 600;
}
.tb-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  --wails-draggable: no-drag;
}
.sb-account {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px 14px 14px;
  margin-bottom: 6px;
  cursor: pointer;
  --wails-draggable: no-drag;
}
.sb-account:hover {
  background: rgba(255, 255, 255, 0.06);
}
.acc-avatar {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  background: var(--accent);
  color: #fff;
  font-size: 15px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.acc-meta {
  display: flex;
  flex-direction: column;
  line-height: 1.4;
  min-width: 0;
}
.acc-name {
  font-size: 13.5px;
  font-weight: 600;
  color: #f3f6fa;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.acc-email {
  font-size: 11.5px;
  color: #93a1b5;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.acc-arrow {
  margin-left: auto;
  color: #93a1b5;
  font-size: 16px;
}
.new-page {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}
.player-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.win-btn {
  width: 36px;
  height: 30px;
  border-radius: 7px;
  color: var(--text-sub);
  font-size: 15px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.12s, color 0.12s;
}
.win-btn:hover {
  background: #eef0f3;
  color: var(--text);
}
.win-btn.close:hover {
  background: var(--danger);
  color: #fff;
}
.new-btn {
  padding: 8px 16px;
  border-radius: 9px;
  font-size: 13.5px;
  font-weight: 500;
  background: var(--accent);
  color: #fff;
}
.new-btn:hover {
  background: #2458e0;
}

.filters {
  display: flex;
  gap: 6px;
  padding: 12px 22px 0;
}
.filter-tab {
  padding: 7px 16px;
  border-radius: 8px;
  font-size: 13.5px;
  color: var(--text-sub);
  background: transparent;
  position: relative;
}
.filter-tab:hover {
  background: #eceff3;
  color: var(--text);
}
.filter-tab.active {
  color: var(--accent);
  font-weight: 600;
}
.filter-tab.active::after {
  content: '';
  position: absolute;
  left: 14px;
  right: 14px;
  bottom: -1px;
  height: 2px;
  border-radius: 1px;
  background: var(--accent);
}

.task-area {
  flex: 1;
  overflow-y: auto;
  padding: 16px 22px 20px;
}
.task-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.task-card {
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
  padding: 10px 14px;
}
.card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.card-title-wrap {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}
.card-title {
  font-size: 13.5px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 320px;
}
.status-badge {
  flex-shrink: 0;
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 999px;
  font-weight: 500;
}
.status-badge.download {
  background: var(--accent-soft);
  color: var(--accent);
}
.status-badge.done {
  background: #e7f8f1;
  color: var(--success);
}
.status-badge.failed {
  background: #fdeaea;
  color: var(--danger);
}
.status-badge.wait {
  background: #f0f2f5;
  color: var(--text-sub);
}
.card-meta-right {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}
.speed {
  color: var(--accent);
  font-size: 12.5px;
  font-weight: 500;
}
.size {
  color: var(--text-sub);
  font-size: 12px;
}
/* 中间行：进度条 + 操作按钮同行 */
.card-mid {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-top: 8px;
}
.progress-wrap {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  min-width: 0;
}
.progress-track {
  flex: 1;
  height: 5px;
  border-radius: 3px;
  background: #eef1f4;
  overflow: hidden;
}
.progress-fill {
  height: 100%;
  border-radius: 3px;
  background: linear-gradient(90deg, #5b8cff, var(--accent));
  transition: width 0.25s ease;
}
.progress-text {
  font-size: 11.5px;
  color: var(--text-sub);
  min-width: 34px;
  text-align: right;
  flex-shrink: 0;
}
/* 底部小字行：错误 / 输出路径 */
.card-sub {
  margin-top: 6px;
  font-size: 11.5px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.card-error {
  color: var(--danger);
}
.card-output {
  color: var(--success);
}

.card-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  flex-shrink: 0;
}
.action-btn {
  padding: 3px 11px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
}
.action-btn.pause {
  background: #eef1f5;
  color: var(--text);
}
.action-btn.pause:hover {
  background: #e2e6ec;
}
.action-btn.resume {
  background: var(--accent-soft);
  color: var(--accent);
}
.action-btn.resume:hover {
  background: #d7e4ff;
}
.action-btn.cancel {
  background: #fdeaea;
  color: var(--danger);
}
.action-btn.cancel:hover {
  background: #fbd8d8;
}
.action-btn.retry {
  background: var(--accent-soft);
  color: var(--accent);
}
.action-btn.retry:hover {
  background: #d7e4ff;
}
.action-btn.del {
  background: #fdeaea;
  color: var(--danger);
}
.action-btn.del:hover {
  background: #fbd8d8;
}

.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--text-faint);
  gap: 8px;
}
.empty-icon {
  font-size: 44px;
  opacity: 0.5;
}
.empty-sub {
  font-size: 12.5px;
  opacity: 0.7;
}

.statbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 22px;
  background: #fff;
  border-top: 1px solid var(--border);
  font-size: 12.5px;
  color: var(--text-sub);
}
.dot {
  color: var(--text-faint);
}
.stat-fail {
  color: var(--danger);
}
.spacer {
  flex: 1;
}
.stat-speed {
  color: var(--accent);
  font-weight: 600;
}
</style>
