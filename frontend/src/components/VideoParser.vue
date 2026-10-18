<script setup>
import { ref } from 'vue'

const inputUrl = ref('')
const playUrl = ref('')
const error = ref('')

// 解析接口配置
const API_BASE = 'http://43.139.76.67:3300/api/bfq/'
const API_KEY = 'J6JP8bMGuDgIqFk6L3'

function play() {
  error.value = ''
  const url = inputUrl.value.trim()
  if (!url) {
    error.value = '请输入视频播放地址'
    return
  }
  if (!/^https?:\/\/.+/i.test(url)) {
    error.value = '地址需以 http:// 或 https:// 开头'
    return
  }
  playUrl.value = `${API_BASE}?key=${API_KEY}&url=${encodeURIComponent(url)}`
}
</script>

<template>
  <div class="parser">
    <div class="parse-card">
      <div class="pc-title">
        <h2>视频解析</h2>
        <p class="pc-sub">输入视频播放地址，点击播放即可在线解析观看</p>
      </div>
      <div class="parse-row">
        <input
          v-model="inputUrl"
          class="input"
          placeholder="https://v.qq.com/x/cover/xxxxx/xxxxx.html"
          spellcheck="false"
          @keyup.enter="play"
        />
        <button class="btn primary" @click="play">播放</button>
      </div>
      <p v-if="error" class="error">{{ error }}</p>
    </div>

    <div v-if="playUrl" class="player-wrap">
      <iframe :src="playUrl" frameborder="0" allowfullscreen allow="autoplay; fullscreen"></iframe>
    </div>
  </div>
</template>

<style scoped>
.parser {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.parse-card {
  max-width: 760px;
  width: 100%;
  margin: 0 auto;
  padding: 28px 28px 0;
  flex-shrink: 0;
  box-sizing: border-box;
}
.pc-title {
  margin-bottom: 16px;
}
.pc-title h2 {
  font-size: 20px;
  font-weight: 600;
}
.pc-sub {
  font-size: 13px;
  color: var(--text-sub);
  margin-top: 6px;
}
.parse-row {
  display: flex;
  gap: 10px;
}
.input {
  flex: 1;
  border: 1px solid var(--border);
  border-radius: 9px;
  padding: 10px 14px;
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
.btn {
  padding: 10px 28px;
  border-radius: 9px;
  font-size: 14px;
  font-weight: 500;
  background: var(--accent);
  color: #fff;
  flex-shrink: 0;
}
.btn:hover {
  background: #2458e0;
}
.error {
  color: var(--danger);
  font-size: 13px;
  margin-top: 10px;
}
.player-wrap {
  flex: 1;
  min-height: 0;
  margin: 20px 28px 24px;
  border-radius: 12px;
  overflow: hidden;
  background: #000;
  border: 1px solid var(--border);
}
.player-wrap iframe {
  width: 100%;
  height: 100%;
  border: none;
  display: block;
}
</style>
