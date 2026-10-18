<script setup>
import { ref, reactive, watch } from 'vue'
import { GetSettings, SaveSettings } from '../../wailsjs/go/main/App'

const props = defineProps({ show: Boolean })
const emit = defineEmits(['close', 'saved'])

const form = reactive({
  apiBase: '',
  apiKey: '',
})
const error = ref('')
const saving = ref(false)

// 弹窗打开时载入当前配置
watch(
  () => props.show,
  async (v) => {
    if (v) {
      error.value = ''
      try {
        const s = await GetSettings()
        form.apiBase = s.apiBaseUrl || ''
        form.apiKey = s.apiKey || ''
      } catch (e) {
        error.value = String(e)
      }
    }
  }
)

async function submit() {
  error.value = ''
  saving.value = true
  try {
    const s = await GetSettings()
    s.apiBaseUrl = form.apiBase.trim()
    s.apiKey = form.apiKey.trim()
    await SaveSettings(s)
    emit('saved')
    emit('close')
  } catch (e) {
    error.value = String(e)
  } finally {
    saving.value = false
  }
}

function onClose() {
  emit('close')
}
</script>

<template>
  <div v-if="show" class="modal-mask" @click.self="onClose">
    <div class="modal account-modal">
      <div class="modal-head">
        <span class="modal-title">账户设置</span>
        <button class="icon-btn" @click="onClose">✕</button>
      </div>

      <div class="field">
        <label>接口地址</label>
        <input
          v-model="form.apiBase"
          class="input"
          placeholder="https://api.example.com"
          spellcheck="false"
        />
        <p class="hint">用于请求资源或服务的 API 地址</p>
      </div>

      <div class="field">
        <label>授权密钥</label>
        <input
          v-model="form.apiKey"
          class="input"
          type="password"
          placeholder="请输入授权密钥"
          spellcheck="false"
        />
        <p class="hint">用于身份验证的密钥，将保存到本地配置</p>
      </div>

      <p v-if="error" class="error">{{ error }}</p>

      <div class="modal-foot">
        <button class="btn ghost" @click="onClose">取消</button>
        <button class="btn primary" :disabled="saving" @click="submit">
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
  width: 440px;
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
  margin-bottom: 16px;
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
.hint {
  font-size: 11.5px;
  color: var(--text-faint);
  margin-top: 5px;
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
