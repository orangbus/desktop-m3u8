<script setup>
defineProps({
  show: Boolean,
  title: { type: String, default: '确认操作' },
  message: { type: String, default: '' },
  confirmText: { type: String, default: '确定' },
  danger: { type: Boolean, default: false },
})
const emit = defineEmits(['confirm', 'cancel'])
</script>

<template>
  <div v-if="show" class="confirm-mask">
    <div class="confirm-dialog">
      <div class="cf-title">{{ title }}</div>
      <div class="cf-message">{{ message }}</div>
      <div class="cf-actions">
        <button class="btn ghost" @click="emit('cancel')">取消</button>
        <button class="btn primary" :class="{ danger }" @click="emit('confirm')">
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
