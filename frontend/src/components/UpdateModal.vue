<script setup>
import { computed } from 'vue'

const props = defineProps({
  show: Boolean,
  info: { type: Object, default: null }, // UpdateInfo
  phase: { type: String, default: 'ready' }, // ready | downloading | installed | saved | error
  progress: { type: Number, default: 0 }, // 0-100
  result: { type: Object, default: null }, // UpdateResult
  error: { type: String, default: '' },
})
const emit = defineEmits(['update', 'github', 'restart', 'opensaved'])

const busy = computed(() => props.phase === 'downloading')
const downloading = computed(() => props.phase === 'downloading')

function fmtBytes(n) {
  if (!n) return ''
  if (n >= 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB'
  if (n >= 1024) return (n / 1024).toFixed(0) + ' KB'
  return n + ' B'
}
</script>

<template>
  <div v-if="show && info" class="update-mask">
    <div class="update-dialog">
      <div class="ud-badge">发现新版本</div>
      <div class="ud-title">
        <span class="ud-ver old">v{{ info.current }}</span>
        <span class="ud-arrow">→</span>
        <span class="ud-ver new">v{{ info.latest }}</span>
      </div>
      <div v-if="info.assetName" class="ud-meta">
        {{ info.assetName }}<template v-if="info.assetSize">（{{ fmtBytes(info.assetSize) }}）</template>
      </div>

      <div class="ud-notes" v-if="info.notes && info.notes.trim()">
        {{ info.notes }}
      </div>

      <!-- 下载进度 -->
      <div v-if="downloading" class="ud-progress">
        <div class="ud-bar"><div class="ud-bar-fill" :style="{ width: progress + '%' }"></div></div>
        <div class="ud-progress-text">正在下载更新包… {{ progress }}%</div>
      </div>

      <!-- Windows/Linux：替换完成，重启生效 -->
      <div v-if="phase === 'installed'" class="ud-done">
        ✓ 更新已就绪，重启应用后生效
      </div>
      <!-- macOS：已下载解压 -->
      <div v-if="phase === 'saved'" class="ud-done">
        ✓ 更新包已下载：{{ result?.path || '' }}
      </div>
      <!-- 失败 -->
      <div v-if="phase === 'error'" class="ud-error">✕ {{ error }}</div>

      <div class="ud-actions">
        <template v-if="phase === 'installed'">
          <button class="btn ghost" @click="emit('github')">前往 GitHub</button>
          <button class="btn primary" @click="emit('restart')">重启应用</button>
        </template>
        <template v-else-if="phase === 'saved'">
          <button class="btn ghost" @click="emit('opensaved')">打开所在文件夹</button>
          <button class="btn primary" @click="emit('restart')">重启应用</button>
        </template>
        <template v-else-if="phase === 'error'">
          <button class="btn ghost" @click="emit('github')">前往 GitHub 下载</button>
          <button class="btn primary" @click="emit('update')">重试在线更新</button>
        </template>
        <template v-else>
          <button class="btn ghost" :disabled="busy" @click="emit('github')">
            前往 GitHub 下载
          </button>
          <button class="btn primary" :disabled="busy" @click="emit('update')">
            {{ busy ? '下载中…' : '立即在线更新' }}
          </button>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.update-mask {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 300; /* 高于普通弹窗，强制置顶 */
  backdrop-filter: blur(3px);
}
.update-dialog {
  background: var(--card);
  border-radius: 14px;
  box-shadow: var(--shadow-lg);
  width: 400px;
  max-width: 90vw;
  padding: 24px 26px;
}
.ud-badge {
  display: inline-block;
  font-size: 12px;
  font-weight: 600;
  color: #fff;
  background: var(--accent);
  border-radius: 999px;
  padding: 3px 12px;
  margin-bottom: 12px;
}
.ud-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 20px;
  font-weight: 700;
  margin-bottom: 6px;
}
.ud-ver.old {
  color: var(--text-sub);
  font-size: 15px;
  text-decoration: line-through;
  opacity: 0.7;
}
.ud-arrow {
  color: var(--text-sub);
  font-size: 15px;
}
.ud-ver.new {
  color: var(--accent);
}
.ud-meta {
  font-size: 12.5px;
  color: var(--text-sub);
  margin-bottom: 12px;
  word-break: break-all;
}
.ud-notes {
  font-size: 13px;
  color: var(--text-sub);
  line-height: 1.6;
  white-space: pre-wrap;
  max-height: 160px;
  overflow-y: auto;
  background: var(--bg, #f5f7fa);
  border-radius: 8px;
  padding: 10px 12px;
  margin-bottom: 14px;
}
.ud-progress {
  margin: 6px 0 14px;
}
.ud-bar {
  height: 8px;
  border-radius: 999px;
  background: #e6e9ee;
  overflow: hidden;
}
.ud-bar-fill {
  height: 100%;
  background: var(--accent);
  border-radius: 999px;
  transition: width 0.2s ease;
}
.ud-progress-text {
  font-size: 12.5px;
  color: var(--text-sub);
  margin-top: 6px;
  text-align: center;
}
.ud-done {
  font-size: 13.5px;
  color: #16a34a;
  margin: 6px 0 4px;
  word-break: break-all;
}
.ud-error {
  font-size: 13.5px;
  color: var(--danger, #e11d48);
  margin: 6px 0 4px;
  word-break: break-all;
}
.ud-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 18px;
}
.btn {
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 13.5px;
  font-weight: 500;
}
.btn.primary {
  background: var(--accent);
  color: #fff;
}
.btn.ghost {
  background: #eef0f3;
  color: var(--text-sub);
}
.btn.ghost:hover {
  background: #e2e5ea;
}
.btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
