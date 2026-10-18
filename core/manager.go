package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Manager 任务调度器：负责同时下载任务数控制、任务状态管理、进度通知。
type Manager struct {
	mu        sync.Mutex
	cond      *sync.Cond
	tasks     map[string]*Task
	settings  Settings
	running   int          // 当前正在下载的任务数
	emit      func(*Task)  // 进度通知回调（推送到前端）
	quit      chan struct{}
	prevBytes map[string]int64 // 速度计算用：上次总字节
}

// NewManager 创建调度器。emit 用于推送任务状态变更到前端。
func NewManager(s Settings, emit func(*Task)) *Manager {
	if emit == nil {
		emit = func(*Task) {}
	}
	m := &Manager{
		tasks:     make(map[string]*Task),
		settings:  s,
		emit:      emit,
		quit:      make(chan struct{}),
		prevBytes: make(map[string]int64),
	}
	m.cond = sync.NewCond(&m.mu)
	return m
}

// ApplySettings 应用新的设置（并发上限即时生效）。
func (m *Manager) ApplySettings(s Settings) {
	m.mu.Lock()
	m.settings = s
	m.cond.Broadcast()
	m.mu.Unlock()
}

// Settings 返回当前设置。
func (m *Manager) Settings() Settings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings
}

// AddTask 添加一个新下载任务，并触发调度。
func (m *Manager) AddTask(urlStr, filename string, headers map[string]string) *Task {
	m.mu.Lock()

	id := uuid.NewString()
	saveDir := m.settings.SavePath
	if saveDir == "" {
		saveDir = filepath.Join(".", "downloads")
	}
	if strings.TrimSpace(filename) == "" {
		filename = time.Now().Format("20060102-150405")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t := &Task{
		ID:        id,
		URL:       urlStr,
		Filename:  filename,
		Headers:   headers,
		Status:    StatusWaiting,
		CreatedAt: nowUnix(),
		dir:       filepath.Join(saveDir, ".tmp", id),
		outDir:    saveDir,
		client:    NewHTTPClient(headers),
		conc:      m.settings.ConcurrencyPerTask,
		ctx:       ctx,
		cancel:    cancel,
	}
	m.tasks[id] = t
	m.emit(t)
	m.cond.Broadcast()
	m.mu.Unlock()

	go m.schedule()
	return t
}

// schedule 后台调度：持续把等待中的任务推入下载（受并发上限约束）。
func (m *Manager) schedule() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for {
		var next *Task
		for _, t := range m.tasks {
			if t.Status == StatusWaiting {
				next = t
				break
			}
		}
		if next == nil {
			return
		}
		for m.running >= m.settings.MaxConcurrentTasks {
			m.cond.Wait()
		}
		if next.Status != StatusWaiting {
			continue
		}
		next.Status = StatusParsing
		m.running++
		m.emit(next)
		go m.runTask(next)
	}
}

// runTask 执行单个任务下载。
func (m *Manager) runTask(t *Task) {
	defer func() {
		m.mu.Lock()
		m.running--
		m.cond.Broadcast()
		// 取消/删除时清理临时目录
		if t.Status == StatusCancelled {
			_ = os.RemoveAll(t.dir)
			delete(m.prevBytes, t.ID)
		}
		m.mu.Unlock()
		go m.schedule()
	}()

	onProgress := func() {
		m.mu.Lock()
		if t.Status == StatusWaiting || t.Status == StatusParsing {
			t.Status = StatusDownload
		}
		m.mu.Unlock()
		m.emit(t)
	}

	out, err := downloadTask(t, onProgress)
	m.mu.Lock()
	if err != nil {
		if err == context.Canceled {
			// 被暂停或取消：保持当前状态（paused/cancelled），不覆盖
		} else {
			t.Status = StatusFailed
			t.Error = err.Error()
		}
	} else {
		t.Status = StatusDone
		t.OutputPath = out
		t.setSpeed(0)
		delete(m.prevBytes, t.ID)
	}
	m.mu.Unlock()
	m.emit(t)
}

// PauseTask 暂停任务（保留已下载分片，可继续）。
func (m *Manager) PauseTask(id string) {
	m.mu.Lock()
	t, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	if t.Status == StatusDownload || t.Status == StatusParsing || t.Status == StatusWaiting {
		t.cancel()
		t.Status = StatusPaused
		t.setSpeed(0)
		delete(m.prevBytes, id)
		m.emit(t)
	}
	m.mu.Unlock()
}

// ResumeTask 继续已暂停的任务（断点续传）。
func (m *Manager) ResumeTask(id string) {
	m.mu.Lock()
	t, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	if t.Status == StatusPaused {
		t.ctx, t.cancel = context.WithCancel(context.Background())
		t.Status = StatusWaiting
		t.Error = ""
		m.cond.Broadcast()
		m.mu.Unlock()
		m.emit(t)
		go m.schedule()
		return
	}
	m.mu.Unlock()
}

// CancelTask 取消任务（清理临时分片，标记为已取消）。
func (m *Manager) CancelTask(id string) {
	m.mu.Lock()
	t, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	if t.Status == StatusDone {
		m.mu.Unlock()
		return
	}
	t.cancel()
	if t.Status != StatusCancelled {
		t.Status = StatusCancelled
		t.Error = ""
		t.setSpeed(0)
		m.emit(t)
	}
	m.mu.Unlock()
}

// RetryTask 重试失败/已取消的任务：清空旧分片后重新加入队列下载。
func (m *Manager) RetryTask(id string) {
	m.mu.Lock()
	t, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	if t.Status != StatusFailed && t.Status != StatusCancelled {
		m.mu.Unlock()
		return
	}
	_ = os.RemoveAll(t.dir)
	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.Status = StatusWaiting
	t.Error = ""
	t.OutputPath = ""
	t.setSpeed(0)
	t.setDoneSegs(0)
	atomic.StoreInt64(&t.TotalBytes, 0)
	m.cond.Broadcast()
	m.mu.Unlock()
	m.emit(t)
	go m.schedule()
}

// RemoveTask 删除任务（若在下载则取消，并清理临时文件与已下载的成品文件）。
// 注意：删除后不再 emit，避免前端把已删除任务重新加回列表。
func (m *Manager) RemoveTask(id string) {
	m.mu.Lock()
	t, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	t.cancel()
	_ = os.RemoveAll(t.dir)
	// 同时删除已下载完成的成品文件
	if t.OutputPath != "" {
		_ = os.Remove(t.OutputPath)
	}
	delete(m.tasks, id)
	delete(m.prevBytes, id)
	m.mu.Unlock()
}

// Tasks 返回全部任务的快照（前端展示）。
func (m *Manager) Tasks() []*Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		c := *t
		c.Speed = atomic.LoadInt64(&t.Speed)
		c.TotalBytes = atomic.LoadInt64(&t.TotalBytes)
		c.DoneSegs = atomic.LoadInt64(&t.DoneSegs)
		list = append(list, &c)
	}
	return list
}

// SpeedLoop 周期计算各下载中任务的速度并推送。
func (m *Manager) SpeedLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.updateSpeeds(interval)
			m.mu.Lock()
			tasks := make([]*Task, 0, len(m.tasks))
			for _, t := range m.tasks {
				if t.Status != StatusDownload {
					continue
				}
				c := *t
				c.Speed = atomic.LoadInt64(&t.Speed)
				c.TotalBytes = atomic.LoadInt64(&t.TotalBytes)
				c.DoneSegs = atomic.LoadInt64(&t.DoneSegs)
				tasks = append(tasks, &c)
			}
			m.mu.Unlock()
			for _, t := range tasks {
				m.emit(t)
			}
		case <-m.quit:
			return
		}
	}
}

func (m *Manager) updateSpeeds(interval time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sec := interval.Seconds()
	for id, t := range m.tasks {
		if t.Status != StatusDownload {
			continue
		}
		cur := atomic.LoadInt64(&t.TotalBytes)
		prev, ok := m.prevBytes[id]
		if !ok {
			prev = cur
		}
		delta := cur - prev
		if delta < 0 {
			delta = 0
		}
		speed := int64(float64(delta) / sec)
		t.setSpeed(speed)
		m.prevBytes[id] = cur
	}
}

// Close 停止调度器。
func (m *Manager) Close() {
	m.mu.Lock()
	select {
	case <-m.quit:
	default:
		close(m.quit)
	}
	m.cond.Broadcast()
	m.mu.Unlock()
}
