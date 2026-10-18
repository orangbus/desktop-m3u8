package core

import (
	"context"
	"sync/atomic"
	"time"
)

// TaskStatus 任务状态
type TaskStatus string

const (
	StatusWaiting   TaskStatus = "waiting"     // 等待调度
	StatusParsing   TaskStatus = "parsing"     // 解析中
	StatusDownload  TaskStatus = "downloading" // 下载中
	StatusPaused    TaskStatus = "paused"      // 已暂停
	StatusDone      TaskStatus = "done"        // 已完成
	StatusFailed    TaskStatus = "failed"      // 失败
	StatusCancelled TaskStatus = "cancelled"   // 已取消
)

// Task 一个下载任务（前端展示的数据结构，经 JSON 序列化）
type Task struct {
	ID         string            `json:"id"`
	URL        string            `json:"url"`
	Filename   string            `json:"filename"`
	Headers    map[string]string `json:"headers"`
	Status     TaskStatus        `json:"status"`
	Progress   float64           `json:"progress"`   // 0-100
	Speed      int64             `json:"speed"`      // 字节/秒（原子更新）
	TotalBytes int64             `json:"totalBytes"` // 已下载字节（原子更新）
	Segments   int               `json:"segments"`   // 总分段数
	DoneSegs   int64             `json:"doneSegs"`   // 已完成分段数（原子更新）
	Error      string            `json:"error"`
	OutputPath string            `json:"outputPath"`
	CreatedAt  int64             `json:"createdAt"`

	// 内部控制字段（不导出到前端）
	ctx    context.Context
	cancel context.CancelFunc
	keyIndex int
	keys     map[int][]byte
	playlist *Playlist
	dir      string // 临时分片目录
	outDir   string // 最终输出目录
	client   *HTTPClient
	conc     int // 单任务分片并发数（由 manager 注入）
}

// addBytes 原子累加已下载字节。
func (t *Task) addBytes(n int64) {
	atomic.AddInt64(&t.TotalBytes, n)
}

// loadBytes 原子读取已下载字节。
func (t *Task) loadBytes() int64 {
	return atomic.LoadInt64(&t.TotalBytes)
}

// setSpeed 原子写入速度。
func (t *Task) setSpeed(bps int64) {
	atomic.StoreInt64(&t.Speed, bps)
}

// loadSpeed 原子读取速度。
func (t *Task) loadSpeed() int64 {
	return atomic.LoadInt64(&t.Speed)
}

// setDoneSegs 原子写入已完成分段数。
func (t *Task) setDoneSegs(n int) {
	atomic.StoreInt64(&t.DoneSegs, int64(n))
}

func nowUnix() int64 {
	return time.Now().Unix()
}
