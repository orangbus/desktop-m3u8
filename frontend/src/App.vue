<script setup>
import { ref, computed, reactive, nextTick, onMounted, onUpdated, onUnmounted } from 'vue'
import { GetTasks, GetSettings, Shutdown, BatchCancel, BatchRemove, PauseTask, ResumeTask, CancelTask, RemoveTask, RetryTask, CopyText, OpenFolder } from '../wailsjs/go/main/App'
import { EventsOn, WindowMinimise, WindowToggleMaximise, Quit } from '../wailsjs/runtime/runtime'
import NewDownloadForm from './components/NewDownloadForm.vue'
import VideoParser from './components/VideoParser.vue'
import SettingsModal from './components/SettingsModal.vue'
import AccountModal from './components/AccountModal.vue'
import ConfirmDialog from './components/ConfirmDialog.vue'
import HelpView from './components/HelpView.vue'

const tasks = ref([])
const filter = ref('downloading')
const keyword = ref('')
const showSettings = ref(false)
const showAccount = ref(false)
const hasLoaded = ref(false)

const removedIds = new Set()

const confirmState = ref({ show: false, id: null, title: '', message: '' })
const closeConfirm = ref(false) // 关闭软件二次确认框
// 批量操作：选中任务 id 集合 + 批量操作确认框
const selectedIds = ref(new Set())
const batchState = ref({ show: false, ids: [], title: '', message: '', fileOption: false, fileOptionLabel: '' })
const selectedCount = computed(() => selectedIds.value.size)
const allSelected = computed(
  () => sortedTasks.value.length > 0 && selectedCount === sortedTasks.value.length
)
function toggleAll() {
  if (allSelected.value) selectedIds.value = new Set()
  else selectedIds.value = new Set(sortedTasks.value.map((t) => t.id))
}
function toggleOne(id, e) {
  const s = new Set(selectedIds.value)
  if (e.target.checked) s.add(id)
  else s.delete(id)
  selectedIds.value = s
}

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
  { key: 'help', label: '使用说明', icon: '？' },
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
  closeConfirm.value = true
}
// 确认关闭：先安全停止所有下载（保留分片）并停掉本地接口服务，再退出进程
async function confirmClose() {
  closeConfirm.value = false
  try {
    await Shutdown()
  } catch (e) {
    /* 即便 Shutdown 失败也继续退出 */
  }
  Quit()
}
function cancelClose() {
  closeConfirm.value = false
}

const filters = [
  { key: 'downloading', label: '下载中' },
  { key: 'done', label: '已完成' },
  { key: 'failed', label: '失败' },
]

const filteredTasks = computed(() => {
  let list
  switch (filter.value) {
    case 'downloading':
      list = tasks.value.filter((t) =>
        ['waiting', 'parsing', 'downloading', 'paused'].includes(t.status)
      )
      break
    case 'done':
      list = tasks.value.filter((t) => t.status === 'done')
      break
    case 'failed':
      list = tasks.value.filter((t) => ['failed', 'cancelled'].includes(t.status))
      break
    default:
      list = tasks.value
  }
  // 关键字搜索：匹配文件名或链接
  const kw = keyword.value.trim().toLowerCase()
  if (kw) {
    list = list.filter(
      (t) =>
        (t.filename || '').toLowerCase().includes(kw) ||
        (t.url || '').toLowerCase().includes(kw)
    )
  }
  return list
})

// 排序：下载中（含等待/解析/暂停）按添加时间正序（最早添加在最前）；
// 已完成/失败/已取消按添加时间倒序（最新在最前）。
const runningKeys = new Set(['waiting', 'parsing', 'downloading', 'paused'])
const sortedTasks = computed(() => {
  const list = filteredTasks.value.slice()
  list.sort((a, b) => {
    const ar = runningKeys.has(a.status)
    const br = runningKeys.has(b.status)
    if (ar !== br) return ar ? -1 : 1 // 下载中整体排在终态之前（兜底，各标签内通常单一类型）
    if (ar) return (a.createdAt || 0) - (b.createdAt || 0) // 下载中：正序，最早在前
    return (b.createdAt || 0) - (a.createdAt || 0) // 完成/失败：倒序，最新在前
  })
  return list
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

// 批量操作：取当前视图下已勾选的任务 id
function selectedTaskIds() {
  return sortedTasks.value
    .filter((t) => selectedIds.value.has(t.id))
    .map((t) => t.id)
}
// 下载中视图：批量取消（正在下载的删分片，未开始的直接取消）
function batchCancel() {
  const ids = selectedTaskIds()
  if (!ids.length) return
  batchState.value = {
    show: true,
    ids,
    title: '批量取消下载',
    message: `确定取消选中的 ${ids.length} 个任务吗？正在下载的任务会同时删除临时分片文件。`,
    fileOption: false,
    fileOptionLabel: '',
  }
}
// 已完成/失败视图：清空记录（提示是否删除本地文件，默认不删除）
function batchClear() {
  const ids = selectedTaskIds()
  if (!ids.length) return
  batchState.value = {
    show: true,
    ids,
    title: '清空记录',
    message: `确定清空选中的 ${ids.length} 条记录吗？默认不删除本地文件。`,
    fileOption: true,
    fileOptionLabel: '同时删除本地文件',
  }
}
async function confirmBatch(opt) {
  const s = batchState.value
  batchState.value.show = false
  const ids = s.ids
  if (!ids.length) return
  try {
    if (filter.value === 'downloading') await BatchCancel(ids)
    else await BatchRemove(ids, !!(opt && opt.deleteFiles))
  } catch (e) {
    /* 静默 */
  }
  selectedIds.value = new Set()
  await loadTasks()
}
function cancelBatch() {
  batchState.value.show = false
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

// 点击链接复制地址（无需提示，点击即复制）
async function copyUrl(url) {
  try {
    await CopyText(url)
  } catch (e) {
    /* 复制失败静默 */
  }
}
// 点击本地路径直接打开所在目录（并选中文件）
function openFolder(path) {
  if (path) OpenFolder(path)
}
// 打开下载保存目录（取自设置 savePath，若为空则打不开时静默）
async function openSaveDir() {
  try {
    const s = await GetSettings()
    const p = (s && s.savePath) || ''
    if (p) OpenFolder(p)
  } catch (e) {
    /* 静默 */
  }
}

// ===== 虚拟滚动：只渲染可视区域内的任务卡片，数百/数千任务也流畅 =====
const listWrap = ref(null) // 滚动容器（.task-area）
const scrollTop = ref(0)
const viewportH = ref(0)
const heights = reactive({}) // 每张卡片高度（按任务 id 缓存）
const EST = 104 // 卡片估算高度(px)，首屏未测量前使用
const GAP = 10 // 卡片间距(px)

// 每个任务占用的“槽位”高度 = 卡片高度 + 间距
function slotOf(t) {
  return (heights[t.id] || EST) + GAP
}

// 当前可视窗口对应的渲染区间：offset 为起始卡片相对列表顶部的位移，totalH 为列表总高
const range = computed(() => {
  const list = sortedTasks.value
  const n = list.length
  if (!n) return { start: 0, end: 0, offset: 0, totalH: 0 }
  const offsets = new Array(n + 1)
  offsets[0] = 0
  for (let i = 0; i < n; i++) offsets[i + 1] = offsets[i] + slotOf(list[i])
  const totalH = Math.max(0, offsets[n] - GAP)
  const st = scrollTop.value
  const bot = st + viewportH.value
  let start = 0
  while (start < n && offsets[start + 1] <= st) start++
  if (start >= n) start = Math.max(0, n - 1)
  let end = start
  while (end < n && offsets[end + 1] < bot) end++
  end = Math.min(n, end + 1)
  if (end <= start) end = start + 1
  return { start, end, offset: offsets[start], totalH }
})

// 只渲染可视区间内的任务
const visibleTasks = computed(() =>
  sortedTasks.value.slice(range.value.start, range.value.end)
)

// 渲染后测量真实卡片高度并缓存（保证精确滚动定位）
async function measure() {
  await nextTick()
  const wrap = listWrap.value
  if (!wrap) return
  const cards = wrap.querySelectorAll('.task-list .task-card')
  const list = sortedTasks.value
  const st = range.value.start
  for (let i = 0; i < cards.length; i++) {
    const idx = st + i
    if (idx < list.length) heights[list[idx].id] = cards[i].offsetHeight
  }
}

function onScroll() {
  scrollTop.value = listWrap.value.scrollTop
}

let resizeObserver = null
function setupViewport() {
  const el = listWrap.value
  if (!el) return
  viewportH.value = el.clientHeight
  resizeObserver = new ResizeObserver(() => {
    viewportH.value = el.clientHeight
  })
  resizeObserver.observe(el)
}

// 新建下载成功后，刷新任务列表并切回「下载管理」
function onAdded() {
  loadTasks()
  currentNav.value = 'download'
}

onMounted(async () => {
  await loadTasks()
  unsubscribe = EventsOn('task:update', (t) => upsertTask(t))
  setupViewport()
  measure()
})

onUpdated(() => {
  measure()
})

onUnmounted(() => {
  if (unsubscribe) unsubscribe()
  if (resizeObserver) resizeObserver.disconnect()
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

      <!-- 使用说明页 -->
      <div v-else-if="currentNav === 'help'" class="help-page">
        <HelpView />
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
          <input
            v-model="keyword"
            class="search-input"
            placeholder="搜索文件名或链接"
            spellcheck="false"
          />
          <button
            class="open-dir-btn"
            title="打开下载目录"
            @click="openSaveDir"
          >
            📁
          </button>
        </nav>

        <!-- 批量操作栏：全选 + 按当前筛选执行批量取消 / 清空记录 -->
        <div v-if="filteredTasks.length" class="batch-bar">
          <label class="batch-all">
            <input
              type="checkbox"
              :checked="allSelected"
              @change="toggleAll"
            />
            <span>全选</span>
          </label>
          <span class="batch-count">已选 {{ selectedCount }}</span>
          <div class="batch-actions">
            <button
              v-if="filter === 'downloading'"
              class="batch-btn"
              :disabled="!selectedCount"
              @click="batchCancel"
            >
              取消下载
            </button>
            <button
              v-else
              class="batch-btn"
              :disabled="!selectedCount"
              @click="batchClear"
            >
              清空记录
            </button>
          </div>
        </div>

        <main class="task-area" ref="listWrap" @scroll="onScroll">
          <div v-if="!hasLoaded" class="empty">加载中…</div>
          <div v-else-if="filteredTasks.length === 0" class="empty">
            <div class="empty-icon">⤓</div>
            <p>暂无该状态下的任务</p>
            <p class="empty-sub">左侧「新建下载」添加任务</p>
          </div>

          <!-- 虚拟滚动：外层高度为列表总高，内层按滚动位移平移，仅渲染可视卡片 -->
          <div v-else class="virtual-list" :style="{ height: range.totalH + 'px' }">
            <div class="task-list" :style="{ transform: 'translateY(' + range.offset + 'px)' }">
              <div v-for="t in visibleTasks" :key="t.id" class="task-card">
              <div class="card-top">
                <div class="card-title-wrap">
                  <label class="card-check" @click.stop>
                    <input
                      type="checkbox"
                      :checked="selectedIds.has(t.id)"
                      @change="toggleOne(t.id, $event)"
                    />
                  </label>
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

              <!-- 下载链接：点击复制地址（无图标标识，上下留间距） -->
              <div class="card-url" title="点击复制链接" @click="copyUrl(t.url)">
                {{ t.url }}
              </div>

              <!-- 已完成：文件路径 + 删除按钮同行（左右布局），点路径打开文件 -->
              <div v-if="t.status === 'done'" class="done-row">
                <span
                  v-if="t.outputPath"
                  class="card-output"
                  title="点击打开文件"
                  @click="openFolder(t.outputPath)"
                >
                  📁 {{ t.outputPath }}
                </span>
                <div class="card-actions">
                  <button class="action-btn del" @click="remove(t.id)">删除</button>
                </div>
              </div>

              <div v-else class="card-mid">
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

              <div v-if="t.error" class="card-sub">
                <span class="card-error">{{ t.error }}</span>
              </div>
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
    <ConfirmDialog
      :show="closeConfirm"
      title="关闭软件"
      message="确定关闭软件吗？将安全停止正在下载的任务，已下载的分片会保留，下次打开后可继续下载。"
      confirm-text="确定关闭"
      @confirm="confirmClose"
      @cancel="cancelClose"
    />
    <ConfirmDialog
      :show="batchState.show"
      :title="batchState.title"
      :message="batchState.message"
      confirm-text="确定"
      :danger="true"
      :file-option="batchState.fileOption"
      :file-option-label="batchState.fileOptionLabel"
      @confirm="confirmBatch"
      @cancel="cancelBatch"
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
  padding: 8px 10px 8px;
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
  padding: 5px 8px 4px;
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
  padding: 12px 14px 10px;
  margin-bottom: 2px;
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
.help-page {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
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
.search-input {
  margin-left: auto;
  width: 220px;
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 6px 12px;
  font-size: 13px;
  color: var(--text);
  background: #fafbfc;
  outline: none;
  transition: border-color 0.15s, background 0.15s;
}
.search-input:focus {
  border-color: var(--accent);
  background: #fff;
}
.search-input::placeholder {
  color: var(--text-faint);
}
.open-dir-btn {
  margin-left: 8px;
  flex-shrink: 0;
  width: 30px;
  height: 30px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--border);
  border-radius: 8px;
  font-size: 15px;
  line-height: 1;
  color: var(--text-sub);
  background: #fafbfc;
  cursor: pointer;
  transition: border-color 0.15s, color 0.15s, background 0.15s;
}
.open-dir-btn:hover {
  border-color: var(--accent);
  color: var(--accent);
  background: #fff;
}

/* 批量操作栏 */
.batch-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 14px;
  margin: 0 16px;
  background: #f6f8fa;
  border: 1px solid var(--border);
  border-radius: 10px;
}
.batch-all {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--text-sub);
  cursor: pointer;
  user-select: none;
}
.batch-all input {
  width: 14px;
  height: 14px;
  accent-color: var(--accent);
  cursor: pointer;
}
.batch-count {
  font-size: 12px;
  color: var(--text-faint);
}
.batch-actions {
  margin-left: auto;
  display: flex;
  gap: 8px;
}
.batch-btn {
  padding: 6px 16px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 500;
  background: var(--accent);
  color: #fff;
  cursor: pointer;
  transition: background 0.15s, opacity 0.15s;
}
.batch-btn:hover:not(:disabled) {
  background: #2458e0;
}
.batch-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
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
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
}
.virtual-list {
  position: relative;
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
.card-check {
  display: inline-flex;
  align-items: center;
  cursor: pointer;
  flex-shrink: 0;
}
.card-check input {
  width: 14px;
  height: 14px;
  accent-color: var(--accent);
  cursor: pointer;
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
/* 链接行：点击复制下载地址（无图标、无底色，仅上下留间距） */
.card-url {
  margin: 8px 0 2px;
  font-size: 12px;
  color: var(--text-sub);
  cursor: pointer;
  user-select: none;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.card-url:hover {
  color: var(--accent);
}
/* 已完成行：文件路径（左）+ 删除按钮（右）同行左右布局 */
.done-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 8px;
}
.done-row .card-output {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.done-row .card-actions {
  flex-shrink: 0;
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
  cursor: pointer;
}
.card-output:hover {
  text-decoration: underline;
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
