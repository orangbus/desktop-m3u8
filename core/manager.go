package core

import (
	"context"
	"encoding/json"
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
	mu         sync.Mutex
	cond       *sync.Cond
	tasks      map[string]*Task
	settings   Settings
	running    int          // 当前正在下载的任务数
	emit       func(*Task)  // 进度通知回调（推送到前端）
	quit       chan struct{}
	prevBytes  map[string]int64 // 速度计算用：上次总字节
	persistFile string         // 任务列表持久化文件路径
	lastSave   time.Time       // 最近一次落盘时间（下载进度做节流）
}

// NewManager 创建调度器。emit 用于推送任务状态变更到前端。
func NewManager(s Settings, emit func(*Task)) *Manager {
	if emit == nil {
		emit = func(*Task) {}
	}
	m := &Manager{
		tasks:       make(map[string]*Task),
		settings:    s,
		emit:        emit,
		quit:        make(chan struct{}),
		prevBytes:   make(map[string]int64),
		persistFile: tasksFilePath(),
	}
	m.cond = sync.NewCond(&m.mu)
	m.load() // 恢复上次会话的任务记录
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
	m.cond.Broadcast()
	m.mu.Unlock()

	// 注意：notify 内部会自行加锁持久化，必须在释放 m.mu 之后再调用，否则死锁。
	m.notify(t)
	go m.schedule()
	return t
}

// schedule 后台调度：持续把等待中的任务推入下载（受并发上限约束）。
func (m *Manager) schedule() {
	for {
		m.mu.Lock()
		var next *Task
		for _, t := range m.tasks {
			if t.Status == StatusWaiting {
				next = t
				break
			}
		}
		if next == nil {
			m.mu.Unlock()
			return
		}
		for m.running >= m.settings.MaxConcurrentTasks {
			m.cond.Wait()
		}
		if next.Status != StatusWaiting {
			m.mu.Unlock()
			continue
		}
		next.Status = StatusParsing
		m.running++
		m.cond.Broadcast()
		m.mu.Unlock()

		// notify 会在内部加锁持久化，必须在释放 m.mu 之后再调用。
		m.notify(next)
		go m.runTask(next)
	}
}

// runTask 执行单个任务下载。
func (m *Manager) runTask(t *Task) {
	defer func() {
		m.mu.Lock()
		m.running--
		m.cond.Broadcast()
		// 保留临时分片供「重试」断点续传（重试只补下缺失分片，不重头开始）。
		// 删除任务时才清理临时目录（见 RemoveTask）。
		delete(m.prevBytes, t.ID)
		m.mu.Unlock()
		go m.schedule()
	}()

	onProgress := func() {
		m.mu.Lock()
		if t.Status == StatusWaiting || t.Status == StatusParsing {
			t.Status = StatusDownload
		}
		m.mu.Unlock()
		m.notify(t)
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
	m.notify(t)
}

// PauseTask 暂停任务（保留已下载分片，可继续）。
func (m *Manager) PauseTask(id string) {
	m.mu.Lock()
	t, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	changed := false
	if t.Status == StatusDownload || t.Status == StatusParsing || t.Status == StatusWaiting {
		t.cancel()
		t.Status = StatusPaused
		t.setSpeed(0)
		delete(m.prevBytes, id)
		changed = true
	}
	m.mu.Unlock()
	if changed {
		m.notify(t)
	}
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
		m.notify(t)
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
	changed := t.Status != StatusCancelled
	if changed {
		t.Status = StatusCancelled
		t.Error = ""
		t.setSpeed(0)
	}
	m.mu.Unlock()
	if changed {
		m.notify(t)
	}
}

// RetryTask 重试失败/已取消的任务。
// 断点续传：保留临时目录中已下载的分片，只补下缺失分片，不重头开始下载。
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
	_ = os.MkdirAll(t.dir, 0o755)
	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.Status = StatusWaiting
	t.Error = ""
	t.OutputPath = ""
	t.setSpeed(0)
	t.setDoneSegs(0)
	// 已下分片计入已下载字节，保证进度条与速度显示准确
	atomic.StoreInt64(&t.TotalBytes, existingBytes(t.dir))
	m.cond.Broadcast()
	m.mu.Unlock()
	m.notify(t)
	go m.schedule()
}

// ShutdownAll 应用安全退出：取消所有未完成任务（保留已下分片供断点续传），
// 并持久化任务记录，供重启后恢复。不删除任何临时文件。
func (m *Manager) ShutdownAll() {
	m.mu.Lock()
	for _, t := range m.tasks {
		switch t.Status {
		case StatusWaiting, StatusParsing, StatusDownload:
			t.cancel()
			t.Status = StatusCancelled
			t.Error = ""
			t.setSpeed(0)
		}
	}
	recs := m.recordsLocked()
	m.mu.Unlock()
	m.writeRecords(recs)
}

// BatchCancel 批量取消下载中/等待中/解析中的任务。
// 正在下载/解析的：取消并删除临时分片文件；尚未开始的（等待中）：直接取消（无分片）。
// 记录保留（标记为已取消），供用户后续重试或删除。
func (m *Manager) BatchCancel(ids []string) {
	m.mu.Lock()
	for _, id := range ids {
		t, ok := m.tasks[id]
		if !ok {
			continue
		}
		switch t.Status {
		case StatusWaiting, StatusParsing, StatusDownload:
			t.cancel()
			_ = os.RemoveAll(t.dir) // 删除临时分片
			t.Status = StatusCancelled
			t.Error = ""
			t.setSpeed(0)
			delete(m.prevBytes, id)
		}
	}
	recs := m.recordsLocked()
	m.mu.Unlock()
	m.writeRecords(recs)
}

// BatchRemove 批量清空任务记录。
// deleteFiles 为 true 时同时删除已下载成品文件与临时分片；为 false 时仅移除记录、保留本地文件。
func (m *Manager) BatchRemove(ids []string, deleteFiles bool) {
	m.mu.Lock()
	for _, id := range ids {
		t, ok := m.tasks[id]
		if !ok {
			continue
		}
		t.cancel()
		if deleteFiles {
			_ = os.RemoveAll(t.dir)
			if t.OutputPath != "" {
				_ = os.Remove(t.OutputPath)
			}
		}
		delete(m.tasks, id)
		delete(m.prevBytes, id)
	}
	recs := m.recordsLocked()
	m.mu.Unlock()
	m.writeRecords(recs)
}

// existingBytes 统计临时分片目录中已下载分片的总字节数（重试续传时回填进度）。
func existingBytes(dir string) int64 {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var total int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total
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
	m.save() // 删除不触发 emit，需主动落盘
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
			// 在锁内直接 emit：保证推送状态与锁内检查一致，与「下载完成/失败」的终态事件
			// 串行化——否则旧的 downloading 快照可能在 done/failed 事件之后才到达前端，
			// 把已完成/失败的任务覆盖回「下载中」，导致列表看起来没有及时刷新。
			// 此处仅推送进度（downloading），持久化由状态变更时的 notify 负责，无需在此写盘。
			for _, t := range tasks {
				if m.emit != nil {
					m.emit(t)
				}
			}
			m.mu.Unlock()
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

// taskRecord 任务的可持久化字段（排除 ctx/cancel/client 等运行时字段）。
type taskRecord struct {
	ID         string            `json:"id"`
	URL        string            `json:"url"`
	Filename   string            `json:"filename"`
	Headers    map[string]string `json:"headers"`
	Status     TaskStatus        `json:"status"`
	TotalBytes int64             `json:"totalBytes"`
	Segments   int               `json:"segments"`
	Error      string            `json:"error"`
	OutputPath string            `json:"outputPath"`
	CreatedAt  int64             `json:"createdAt"`
}

// tasksFilePath 任务列表持久化文件路径（与 settings.json 同目录）。
func tasksFilePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "desk-m3u8-tasks.json"
	}
	dir = filepath.Join(dir, "desk-m3u8")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "tasks.json")
}

// recordsLocked 将内存任务转成可持久化记录（调用方需持有 m.mu）。
func (m *Manager) recordsLocked() []taskRecord {
	list := make([]taskRecord, 0, len(m.tasks))
	for _, t := range m.tasks {
		list = append(list, taskRecord{
			ID:         t.ID,
			URL:        t.URL,
			Filename:   t.Filename,
			Headers:    t.Headers,
			Status:     t.Status,
			TotalBytes: atomic.LoadInt64(&t.TotalBytes),
			Segments:   t.Segments,
			Error:      t.Error,
			OutputPath: t.OutputPath,
			CreatedAt:  t.CreatedAt,
		})
	}
	return list
}

// writeRecords 写入磁盘（不做任何加锁，调用方负责）。
func (m *Manager) writeRecords(recs []taskRecord) {
	data, err := json.Marshal(recs)
	if err != nil {
		return
	}
	_ = os.WriteFile(m.persistFile, data, 0o644)
}

// notify 推送状态变更到前端并持久化任务列表。
// 下载进行中的高频进度更新做节流（每 2 秒一次），终态/等待等立即保存。
func (m *Manager) notify(t *Task) {
	if m.emit != nil {
		m.emit(t)
	}
	progressOnly := t.Status == StatusDownload
	m.mu.Lock()
	if progressOnly {
		if time.Since(m.lastSave) < 2*time.Second {
			m.mu.Unlock()
			return
		}
		m.lastSave = time.Now()
	}
	recs := m.recordsLocked()
	m.mu.Unlock()
	m.writeRecords(recs)
}

// save 立即将任务列表写入磁盘（供不触发 emit 的变更，如删除任务）。
func (m *Manager) save() {
	m.mu.Lock()
	recs := m.recordsLocked()
	m.mu.Unlock()
	m.writeRecords(recs)
}

// load 启动时从磁盘恢复上次会话的任务记录。
// - 已完成/已暂停/失败/已取消：按原状态恢复（分片与成品仍在磁盘）。
// - 下载中/解析中/等待中：会话中断无法自动续跑，统一置为失败，
//   用户可点「重试」走断点续传（只补缺失分片）。
func (m *Manager) load() {
	data, err := os.ReadFile(m.persistFile)
	if err != nil {
		return
	}
	var recs []taskRecord
	if err := json.Unmarshal(data, &recs); err != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	saveDir := m.settings.SavePath
	if saveDir == "" {
		saveDir = filepath.Join(".", "downloads")
	}
	for _, r := range recs {
		if _, ok := m.tasks[r.ID]; ok {
			continue
		}
		status := r.Status
		switch status {
		case StatusWaiting, StatusParsing, StatusDownload:
			status = StatusFailed
			if r.Error == "" {
				r.Error = "上次会话中断，可重试续传"
			}
		}
		if r.Headers == nil {
			r.Headers = map[string]string{}
		}
		t := &Task{
			ID:         r.ID,
			URL:        r.URL,
			Filename:   r.Filename,
			Headers:    r.Headers,
			Status:     status,
			TotalBytes: r.TotalBytes,
			Segments:   r.Segments,
			Error:      r.Error,
			OutputPath: r.OutputPath,
			CreatedAt:  r.CreatedAt,
			dir:        filepath.Join(saveDir, ".tmp", r.ID),
			outDir:     saveDir,
			client:     NewHTTPClient(r.Headers),
			conc:       m.settings.ConcurrencyPerTask,
			ctx:        context.Background(),
			cancel:     func() {},
		}
		m.tasks[r.ID] = t
	}
}
