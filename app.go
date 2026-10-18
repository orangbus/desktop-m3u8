package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"desk-m3u8/core"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// apiDownloadPort 本地 HTTP 接收接口端口（仅监听 127.0.0.1）。
const apiDownloadPort = "52323"

// apiPayload 浏览器推送的单个任务。
type apiPayload struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// App 是 Wails 绑定的应用结构。
type App struct {
	ctx     context.Context
	manager *core.Manager
}

// NewApp 创建 App 实例。
func NewApp() *App {
	return &App{}
}

// startup 应用启动时初始化调度器并绑定进度推送。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	settings := core.LoadSettings()
	a.manager = core.NewManager(settings, a.emitTask)
	go a.manager.SpeedLoop(1 * time.Second)
	a.startHTTPAPI()
}

// emitTask 推送任务状态变更到前端。
func (a *App) emitTask(t *core.Task) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "task:update", t)
}

// startHTTPAPI 启动本地 HTTP 服务，接收浏览器 POST 推送的下载任务。
// 接口：POST http://127.0.0.1:9210/api/download
// 支持单个 {name,url} 或批量 [{name,url},...]。
func (a *App) startHTTPAPI() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/download", a.handleDownload)
	srv := &http.Server{
		Addr:    "127.0.0.1:" + apiDownloadPort,
		Handler: cors(mux),
	}
	go func() {
		_ = srv.ListenAndServe()
	}()
}

// cors 允许浏览器跨域访问本地接口。
func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// handleDownload 处理 /api/download 请求，把任务加入下载队列。
func (a *App) handleDownload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodPost {
		http.Error(w, `{"code":1,"message":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if a.manager == nil {
		http.Error(w, `{"code":1,"message":"manager not ready"}`, http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"code":1,"message":"read body failed"}`, http.StatusBadRequest)
		return
	}

	// 先尝试批量（数组），失败则回退为单个对象
	var batch []apiPayload
	if err := json.Unmarshal(body, &batch); err != nil {
		var one apiPayload
		if err := json.Unmarshal(body, &one); err != nil {
			http.Error(w, `{"code":1,"message":"invalid json, expect {name,url} or [{name,url},...]"}`, http.StatusBadRequest)
			return
		}
		batch = []apiPayload{one}
	}

	count := 0
	rejected := 0
	for _, p := range batch {
		u := trim(p.URL)
		if u == "" {
			rejected++
			continue
		}
		a.manager.AddTask(u, trim(p.Name), nil)
		count++
	}
	resp := map[string]interface{}{
		"code":     0,
		"message":  "ok",
		"count":    count,
		"rejected": rejected,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// AddDownload 添加一个新的下载任务。headers 为请求头键值对。
func (a *App) AddDownload(url, filename string, headers map[string]string) *core.Task {
	if a.manager == nil {
		return nil
	}
	return a.manager.AddTask(url, filename, headers)
}

// GetTasks 返回全部任务。
func (a *App) GetTasks() []*core.Task {
	if a.manager == nil {
		return []*core.Task{}
	}
	return a.manager.Tasks()
}

// PauseTask 暂停任务。
func (a *App) PauseTask(id string) {
	if a.manager != nil {
		a.manager.PauseTask(id)
	}
}

// ResumeTask 继续已暂停的任务。
func (a *App) ResumeTask(id string) {
	if a.manager != nil {
		a.manager.ResumeTask(id)
	}
}

// CancelTask 取消任务。
func (a *App) CancelTask(id string) {
	if a.manager != nil {
		a.manager.CancelTask(id)
	}
}

// RemoveTask 删除任务。
func (a *App) RemoveTask(id string) {
	if a.manager != nil {
		a.manager.RemoveTask(id)
	}
}

// RetryTask 重试失败/已取消的任务。
func (a *App) RetryTask(id string) {
	if a.manager != nil {
		a.manager.RetryTask(id)
	}
}

// GetSettings 返回当前设置。
func (a *App) GetSettings() core.Settings {
	if a.manager == nil {
		return core.DefaultSettings()
	}
	return a.manager.Settings()
}

// SaveSettings 保存设置。
func (a *App) SaveSettings(s core.Settings) error {
	if err := core.SaveSettings(s); err != nil {
		return err
	}
	if a.manager != nil {
		a.manager.ApplySettings(s)
	}
	return nil
}

// ChooseFolder 打开系统文件夹选择对话框，返回所选路径（取消则返回空串）。
func (a *App) ChooseFolder() string {
	if a.ctx == nil {
		return ""
	}
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择保存路径",
	})
	if err != nil {
		return ""
	}
	return dir
}
