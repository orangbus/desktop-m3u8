<script setup>
import { reactive, ref, watch } from 'vue'
import { GetSettings, SaveSettings, ChooseFolder } from '../../wailsjs/go/main/App'

const props = defineProps({ show: Boolean })
const emit = defineEmits(['close', 'saved'])

const form = reactive({
  maxConcurrentTasks: 3,
  concurrencyPerTask: 10,
  savePath: '',
  apiPort: 52323,
})
const error = ref('')
const saving = ref(false)

watch(
  () => props.show,
  async (val) => {
    if (val) {
      error.value = ''
      const s = await GetSettings()
      form.maxConcurrentTasks = s.maxConcurrentTasks
      form.concurrencyPerTask = s.concurrencyPerTask
      form.savePath = s.savePath || ''
      form.apiPort = s.apiPort || 52323
    }
  }
)

async function pickFolder() {
  const dir = await ChooseFolder()
  if (dir) form.savePath = dir
}

async function save() {
  error.value = ''
  saving.value = true
  try {
    await SaveSettings({
      maxConcurrentTasks: form.maxConcurrentTasks,
      concurrencyPerTask: form.concurrencyPerTask,
      savePath: form.savePath,
      apiPort: Number(form.apiPort) || 52323,
    })
    emit('saved')
    emit('close')
  } catch (e) {
    error.value = String(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div v-if="show" class="modal-mask" @click.self="emit('close')">
    <div class="modal settings">
      <div class="modal-head">
        <span class="modal-title">设置</span>
        <button class="icon-btn" @click="emit('close')">✕</button>
      </div>

      <div class="row">
        <div class="row-label">
          <div class="row-title">同时下载的任务数</div>
          <div class="row-sub">最多同时有几个任务在下载</div>
        </div>
        <div class="stepper">
          <button class="step-btn" @click="form.maxConcurrentTasks = Math.max(1, form.maxConcurrentTasks - 1)">−</button>
          <input v-model.number="form.maxConcurrentTasks" class="num-input" type="number" min="1" />
          <button class="step-btn" @click="form.maxConcurrentTasks = Math.min(20, form.maxConcurrentTasks + 1)">+</button>
        </div>
      </div>

      <div class="row">
        <div class="row-label">
          <div class="row-title">单个任务并发数量</div>
          <div class="row-sub">单个任务同时下载的分片数</div>
        </div>
        <div class="stepper">
          <button class="step-btn" @click="form.concurrencyPerTask = Math.max(1, form.concurrencyPerTask - 1)">−</button>
          <input v-model.number="form.concurrencyPerTask" class="num-input" type="number" min="1" />
          <button class="step-btn" @click="form.concurrencyPerTask = Math.min(64, form.concurrencyPerTask + 1)">+</button>
        </div>
      </div>

      <div class="row">
        <div class="row-label">
          <div class="row-title">文件保存路径</div>
          <div class="row-sub">下载视频的存放目录</div>
        </div>
        <div class="path-picker">
          <input v-model="form.savePath" class="input" placeholder="选择或输入保存目录" spellcheck="false" />
          <button class="folder-btn" @click="pickFolder">📁</button>
        </div>
      </div>

      <div class="row">
        <div class="row-label">
          <div class="row-title">API 下载端口</div>
          <div class="row-sub">接口提交下载的本地监听端口（1-65535，保存后立即生效）</div>
        </div>
        <input v-model.number="form.apiPort" class="port-input" type="number" min="1" max="65535" placeholder="52323" />
      </div>

      <p v-if="error" class="error">{{ error }}</p>

      <div class="modal-foot">
        <button class="btn primary" :disabled="saving" @click="save">
          {{ saving ? '保存中…' : '保存' }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.35);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
  backdrop-filter: blur(2px);
}
.modal {
  background: var(--card);
  border-radius: 16px;
  box-shadow: var(--shadow-lg);
  width: 520px;
  max-width: 92vw;
  padding: 22px 24px;
}
.modal-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}
.modal-title {
  font-size: 17px;
  font-weight: 600;
}
.icon-btn {
  width: 26px;
  height: 26px;
  border-radius: 6px;
  color: var(--text-faint);
  font-size: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.icon-btn:hover {
  background: var(--border);
  color: var(--text);
}
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 0;
  border-bottom: 1px solid var(--border);
}
.row:last-of-type {
  border-bottom: none;
}
.row-title {
  font-size: 14px;
  font-weight: 500;
}
.row-sub {
  font-size: 12px;
  color: var(--text-faint);
  margin-top: 3px;
}
.stepper {
  display: flex;
  align-items: center;
  gap: 6px;
}
.step-btn {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  background: #f0f2f5;
  color: var(--text);
  font-size: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.step-btn:hover {
  background: #e2e5ea;
}
.num-input {
  width: 52px;
  text-align: center;
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 6px 4px;
  font-size: 14px;
  font-weight: 600;
  outline: none;
}
.num-input:focus {
  border-color: var(--accent);
}
.port-input {
  width: 110px;
  text-align: center;
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 8px 10px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
  background: #fafbfc;
  outline: none;
}
.port-input:focus {
  border-color: var(--accent);
  background: #fff;
}
.path-picker {
  display: flex;
  gap: 8px;
  align-items: center;
  width: 60%;
}
.input {
  flex: 1;
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 13px;
  color: var(--text);
  background: #fafbfc;
  outline: none;
}
.input:focus {
  border-color: var(--accent);
  background: #fff;
}
.folder-btn {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  background: #f0f2f5;
  font-size: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.folder-btn:hover {
  background: #e2e5ea;
}
.error {
  color: var(--danger);
  font-size: 12.5px;
  margin: 8px 0 0;
}
.modal-foot {
  display: flex;
  justify-content: flex-end;
  margin-top: 14px;
}
.btn {
  padding: 9px 24px;
  border-radius: 8px;
  font-size: 13.5px;
  font-weight: 500;
}
.btn.primary {
  background: var(--accent);
  color: #fff;
}
.btn.primary:hover:not(:disabled) {
  background: #2458e0;
}
</style>
