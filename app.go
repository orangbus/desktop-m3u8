package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	goruntime "runtime"
	"sync"
	"time"

	"desk-m3u8/core"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// defaultAPIPort 本地接口默认监听端口（未配置或非法时回退），仅监听 127.0.0.1。
// 可在「设置」中自定义端口。
const defaultAPIPort = 52323

// apiPayload 浏览器推送的单个任务。
type apiPayload struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// App 是 Wails 绑定的应用结构。
type App struct {
	ctx      context.Context
	manager  *core.Manager
	httpMu   sync.Mutex // 保护 httpSrv / httpPort
	httpSrv  *http.Server
	httpPort int
}

// NewApp 创建 App 实例。
func NewApp() *App {
	return &App{}
}

// startup 应用启动时初始化调度器并绑定进度推送。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Wails 的 EventsEmit 会同时通知「Go 侧监听器」与前端；若 Go 侧没有注册
	// task:update 的监听器，每次推送都会输出 "No listeners for event 'task:update'"
	// 的 Trace 日志。注册一个占位监听器即可消除（前端已用 EventsOn 订阅，推送仍正常到达）。
	runtime.EventsOn(ctx, "task:update", func(...interface{}) {})
	settings := core.LoadSettings()
	a.manager = core.NewManager(settings, a.emitTask)
	go a.manager.SpeedLoop(1 * time.Second)
	a.startHTTPAPI(apiPortOf(settings))
}

// emitTask 推送任务状态变更到前端。
func (a *App) emitTask(t *core.Task) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "task:update", t)
}

// apiPortOf 返回端口设置的有效值，非法时回退默认端口。
func apiPortOf(s core.Settings) int {
	if s.ApiPort >= 1 && s.ApiPort <= 65535 {
		return s.ApiPort
	}
	return defaultAPIPort
}

// startHTTPAPI 启动本地 HTTP 服务，接收浏览器 POST 推送的下载任务。
// 接口：POST http://127.0.0.1:<端口>/api/download（端口由设置决定，默认 52323）
// 支持单个 {name,url} 或批量 [{name,url},...]。端口变化时自动重启服务。
func (a *App) startHTTPAPI(port int) {
	a.httpMu.Lock()
	defer a.httpMu.Unlock()
	// 端口未变化且服务已启动则跳过（幂等）
	if a.httpSrv != nil && a.httpPort == port {
		return
	}
	if a.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		_ = a.httpSrv.Shutdown(ctx)
		cancel()
	}
	a.httpPort = port
	mux := http.NewServeMux()
	mux.HandleFunc("/api/download", a.handleDownload)
	srv := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: cors(mux),
	}
	a.httpSrv = srv
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

// Shutdown 应用安全退出：取消所有正在下载的任务（保留已下分片可续传）、停止本地 HTTP 服务。
// 前端在关闭确认框点「确定」后调用，随后调用 Quit() 真正退出进程。
func (a *App) Shutdown() {
	if a.manager != nil {
		a.manager.ShutdownAll()
	}
	a.httpMu.Lock()
	if a.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		_ = a.httpSrv.Shutdown(ctx)
		cancel()
		a.httpSrv = nil
	}
	a.httpMu.Unlock()
}

// BatchCancel 批量取消下载中的任务（正在下载的删除分片、未开始的直接取消），记录保留。
func (a *App) BatchCancel(ids []string) {
	if a.manager != nil {
		a.manager.BatchCancel(ids)
	}
}

// BatchRemove 批量清空任务记录；deleteFiles 为 true 时同时删除本地成品文件与分片。
func (a *App) BatchRemove(ids []string, deleteFiles bool) {
	if a.manager != nil {
		a.manager.BatchRemove(ids, deleteFiles)
	}
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
	// 端口变化时重启本地接口服务
	a.startHTTPAPI(apiPortOf(s))
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

// CopyText 复制文本到系统剪贴板（下载列表中点击链接复制地址）。
func (a *App) CopyText(text string) {
	if a.ctx == nil {
		return
	}
	_ = runtime.ClipboardSetText(a.ctx, text)
}

// OpenFolder 打开本地文件或目录（跨平台）：文件用系统默认程序打开，目录直接打开。
// 下载完成后点击路径调用；path 为成品文件完整路径。
func (a *App) OpenFolder(path string) {
	if path == "" {
		return
	}
	switch goruntime.GOOS {
	case "windows":
		openOnWindows(path)
	case "darwin":
		// 用系统默认程序打开（文件或目录）
		_ = exec.Command("open", path).Start()
	default:
		// Linux：用系统默认程序打开
		_ = exec.Command("xdg-open", path).Start()
	}
}

// openOnWindows 在 Windows 上用系统默认方式打开本地文件/目录（ShellExecute）。
// 目录交给资源管理器，文件交给默认程序。统一走 cmd /c start，对含空格路径可靠
// （explorer /select、rundll32 ShellExec_RunDLL 在某些环境易静默失败）。
func openOnWindows(p string) {
	_ = exec.Command("cmd", "/c", "start", "", p).Start()
}
