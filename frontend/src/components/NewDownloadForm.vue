<script setup>
import { reactive, ref } from 'vue'
import { AddDownload } from '../../wailsjs/go/main/App'

const emit = defineEmits(['added'])

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
</script>

<template>
  <div class="new-form">
    <div class="nf-head">
      <h2 class="nf-title">新建下载</h2>
      <p class="nf-sub">输入 M3U8 地址，可自定义文件名与请求头</p>
    </div>

    <div class="nf-card">
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

      <div class="nf-foot">
        <button class="btn primary" :disabled="submitting" @click="submit">
          {{ submitting ? '添加中…' : '开始下载' }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.new-form {
  max-width: 720px;
  width: 100%;
  margin: 0 auto;
  padding: 40px 28px 48px;
}
.nf-head {
  margin-bottom: 26px;
}
.nf-title {
  font-size: 21px;
  font-weight: 600;
}
.nf-sub {
  font-size: 13.5px;
  color: var(--text-sub);
  margin-top: 7px;
}
.nf-card {
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: 14px;
  box-shadow: var(--shadow);
  padding: 30px 34px 26px;
}
.field {
  margin-bottom: 22px;
}
.field label {
  display: block;
  font-size: 13px;
  color: var(--text-sub);
  margin-bottom: 9px;
}
.input {
  width: 100%;
  border: 1px solid var(--border);
  border-radius: 9px;
  padding: 11px 14px;
  font-size: 14px;
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
  border-radius: 9px;
  padding: 14px;
  background: #fafbfc;
}
.header-row {
  display: flex;
  gap: 10px;
  margin-bottom: 10px;
  align-items: center;
}
.header-row:last-of-type {
  margin-bottom: 0;
}
.h-key {
  width: 34%;
}
.h-val {
  flex: 1;
}
.icon-btn {
  width: 28px;
  height: 28px;
  border-radius: 7px;
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
.add-header {
  margin-top: 12px;
  color: var(--accent);
  font-size: 13px;
}
.error {
  color: var(--danger);
  font-size: 13px;
  margin: 10px 0 0;
}
.nf-foot {
  display: flex;
  justify-content: flex-end;
  margin-top: 26px;
}
.btn {
  padding: 11px 30px;
  border-radius: 9px;
  font-size: 14px;
  font-weight: 500;
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
</style>
