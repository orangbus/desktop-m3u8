<script setup>
import { ref, watch } from 'vue'
const props = defineProps({
  show: Boolean,
  title: { type: String, default: '确认操作' },
  message: { type: String, default: '' },
  confirmText: { type: String, default: '确定' },
  danger: { type: Boolean, default: false },
  fileOption: { type: Boolean, default: false }, // 是否显示「同时删除本地文件」勾选项
  fileOptionLabel: { type: String, default: '同时删除本地文件' },
})
const emit = defineEmits(['confirm', 'cancel'])
const deleteFiles = ref(false)
// 每次打开默认不勾选「删除本地文件」
watch(
  () => props.show,
  (v) => {
    if (v) deleteFiles.value = false
  }
)
function onConfirm() {
  emit('confirm', { deleteFiles: deleteFiles.value })
}
</script>

<template>
  <div v-if="show" class="confirm-mask">
    <div class="confirm-dialog">
      <div class="cf-title">{{ title }}</div>
      <div class="cf-message">{{ message }}</div>
      <label v-if="fileOption" class="cf-file-opt">
        <input type="checkbox" v-model="deleteFiles" />
        <span>{{ fileOptionLabel }}</span>
      </label>
      <div class="cf-actions">
        <button class="btn ghost" @click="emit('cancel')">取消</button>
        <button class="btn primary" :class="{ danger }" @click="onConfirm">
          {{ confirmText }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.confirm-mask {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.35);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 200;
  backdrop-filter: blur(2px);
}
.confirm-dialog {
  background: var(--card);
  border-radius: 14px;
  box-shadow: var(--shadow-lg);
  width: 340px;
  max-width: 88vw;
  padding: 22px 24px;
}
.cf-title {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 10px;
}
.cf-message {
  font-size: 13.5px;
  color: var(--text-sub);
  line-height: 1.6;
  word-break: break-all;
}
.cf-file-opt {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 14px;
  font-size: 13px;
  color: var(--text-sub);
  cursor: pointer;
  user-select: none;
}
.cf-file-opt input {
  accent-color: var(--accent);
  width: 14px;
  height: 14px;
}
.cf-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 20px;
}
.btn {
  padding: 8px 18px;
  border-radius: 8px;
  font-size: 13.5px;
  font-weight: 500;
}
.btn.primary {
  background: var(--accent);
  color: #fff;
}
.btn.primary.danger {
  background: var(--danger);
}
.btn.primary.danger:hover {
  background: #d63a4b;
}
.btn.ghost {
  background: #eef0f3;
  color: var(--text-sub);
}
.btn.ghost:hover {
  background: #e2e5ea;
}
</style>
