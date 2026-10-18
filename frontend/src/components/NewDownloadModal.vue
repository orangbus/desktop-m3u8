<script setup>
import { reactive, ref } from 'vue'
import { AddDownload } from '../../wailsjs/go/main/App'

const props = defineProps({ show: Boolean })
const emit = defineEmits(['close', 'added'])

const form = reactive({
  url: '',
  filename: '',
})
const headers = ref([]) // [{key, value}]
const error = ref('')
const submitting = ref(false)

function addHeader() {
  headers.value.push({ key: '', value: '' })
}
function removeHeader(i) {
  headers.value.splice(i, 1)
}

function buildHeaders() {
  const h = {}
  for (const item of headers.value) {
    const k = item.key.trim()
    const v = item.value.trim()
    if (k && v) h[k] = v
  }
  return h
}

async function submit() {
  error.value = ''
  const url = form.url.trim()
  if (!url) {
    error.value = '请输入 M3U8 地址'
    return
  }
  if (!/^https?:\/\/.+/i.test(url)) {
    error.value = '地址需以 http:// 或 https:// 开头'
    return
  }
  submitting.value = true
  try {
    const filename = form.filename.trim() || ''
    await AddDownload(url, filename, buildHeaders())
    reset()
    emit('added')
    emit('close') // 添加成功后关闭弹窗
  } catch (e) {
    error.value = String(e)
  } finally {
    submitting.value = false
  }
}

function reset() {
  form.url = ''
  form.filename = ''
  headers.value = []
  error.value = ''
}

function onClose() {
  emit('close')
}
</script>

<template>
  <div v-if="show" class="modal-mask" @click.self="onClose">
    <div class="modal new-dl">
      <div class="modal-head">
        <span class="modal-title">新建下载</span>
        <button class="icon-btn" @click="onClose">✕</button>
      </div>

      <div class="field">
        <label>M3U8 地址</label>
        <input
          v-model="form.url"
          class="input"
          placeholder="https://example.com/index.m3u8"
          spellcheck="false"
        />
      </div>

      <div class="field">
        <label>文件名</label>
        <input
          v-model="form.filename"
          class="input"
          placeholder="留空则使用默认文件名"
          spellcheck="false"
        />
      </div>

      <div class="field">
        <label>请求头 Headers</label>
        <div class="headers">
          <div v-for="(item, i) in headers" :key="i" class="header-row">
            <input v-model="item.key" class="input h-key" placeholder="User-Agent" spellcheck="false" />
            <input v-model="item.value" class="input h-val" placeholder="值" spellcheck="false" />
            <button class="icon-btn" @click="removeHeader(i)">✕</button>
          </div>
          <button v-if="headers.length === 0" class="add-header" @click="addHeader">
            + 添加请求头
          </button>
          <button v-else class="add-header" @click="addHeader">+ 添加</button>
        </div>
      </div>

      <p v-if="error" class="error">{{ error }}</p>

      <div class="modal-foot">
        <button class="btn ghost" @click="onClose">取消</button>
        <button class="btn primary" :disabled="submitting" @click="submit">
          {{ submitting ? '添加中…' : '开始下载' }}
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
  width: 480px;
  max-width: 92vw;
  padding: 22px 24px;
}
.modal-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 18px;
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
.field {
  margin-bottom: 14px;
}
.field label {
  display: block;
  font-size: 12.5px;
  color: var(--text-sub);
  margin-bottom: 6px;
}
.input {
  width: 100%;
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 9px 12px;
  font-size: 13.5px;
  color: var(--text);
  background: #fafbfc;
  outline: none;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.input:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
  background: #fff;
}
.headers {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 10px;
  background: #fafbfc;
}
.header-row {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
  align-items: center;
}
.header-row:last-of-type {
  margin-bottom: 0;
}
.h-key {
  width: 32%;
}
.h-val {
  flex: 1;
}
.add-header {
  margin-top: 10px;
  color: var(--accent);
  font-size: 12.5px;
}
.error {
  color: var(--danger);
  font-size: 12.5px;
  margin: 8px 0 0;
}
.modal-foot {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 18px;
}
.btn {
  padding: 9px 20px;
  border-radius: 8px;
  font-size: 13.5px;
  font-weight: 500;
  transition: opacity 0.15s;
}
.btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.btn.primary {
  background: var(--accent);
  color: #fff;
}
.btn.primary:hover:not(:disabled) {
  background: #2458e0;
}
.btn.ghost {
  background: #eef0f3;
  color: var(--text-sub);
}
.btn.ghost:hover {
  background: #e2e5ea;
}
</style>
