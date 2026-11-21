package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	goruntime "runtime"
	"strings"
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
	// 清理上次在线更新遗留的旧程序备份，并异步检查新版本
	cleanupOldBinary()
	go func() {
		time.Sleep(1200 * time.Millisecond) // 等窗口与前端就绪后再推送事件
		if info := a.checkUpdate(); info != nil && info.HasUpdate {
			runtime.EventsEmit(ctx, "update:available", info)
		}
	}()
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
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// 端口被占用等启动失败不能静默：打日志便于排查
			fmt.Printf("[api] 本地接口启动失败(端口 %d): %v\n", port, err)
		}
	}()
}

// cors 允许浏览器跨域访问本地接口。
func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 浏览器私有网络访问（PNA/LNA）对 loopback 请求要求精确匹配 Origin；
		// 有 Origin 时原样反射（等价于原先的 *，不降低安全性），无 Origin（curl 等）时回退 *。
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Api-Key, Authorization")
		// 关键：Chrome/Edge 把「公网页面 → 127.0.0.1」视为跨地址空间请求，
		// 预检响应必须带此头，否则报 "No 'Access-Control-Allow-Private-Network' header"。
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// maxAPIBody 本地接口请求体大小上限（1MB，足够上千条批量推送）。
const maxAPIBody = 1 << 20

// handleDownload 处理 /api/download 请求，把任务加入下载队列。
// 支持三种提交方式：
//   - POST JSON：{name,url} 或 [{name,url},...]（程序化调用，需处理 CORS/LNA）
//   - POST 表单：url=...&name=...（导航提交，不受 LNA 限制）
//   - GET 链接：?url=...&name=...（地址栏直接打开 / 书签，不受 LNA 限制，最省事）
//
// 若设置中配置了授权密钥（apiKey），GET/表单可通过 ?key= / 表单 key= 传入，
// 或用 X-Api-Key 头 / Authorization: Bearer，未配置则不校验。
func (a *App) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		http.Error(w, `{"code":1,"message":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if a.manager == nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		http.Error(w, `{"code":1,"message":"manager not ready"}`, http.StatusServiceUnavailable)
		return
	}
	// 授权密钥校验（未配置则不校验，保持开箱即用）
	if key := trim(a.manager.Settings().ApiKey); key != "" {
		got := r.Header.Get("X-Api-Key")
		if got == "" {
			got = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		if got == "" {
			got = r.URL.Query().Get("key") // GET 导航提交无法自定义头，用查询参数
		}
		if got != key {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			http.Error(w, `{"code":1,"message":"unauthorized: missing or invalid api key"}`, http.StatusUnauthorized)
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAPIBody)
	batch, err := parseAPIPayloads(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		http.Error(w, `{"code":1,"message":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
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

	// GET / 表单属于导航请求：返回结果页，可自动跳回来源页，避免浏览器停在裸 JSON 上。
	if isNavSubmit(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		ref := r.URL.Query().Get("back")
		if ref == "" {
			ref = r.Header.Get("Referer")
		}
		fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>已提交</title>
<style>body{font:14px/1.6 system-ui,sans-serif;text-align:center;padding:60px;color:#333}</style>
<p>已提交 <b>%d</b> 个下载任务，可关闭本页。</p>`, count)
		if ref != "" {
			fmt.Fprintf(w, `<script>setTimeout(function(){location.href=%q},1200)</script>`, ref)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	resp := map[string]interface{}{
		"code":     0,
		"message":  "ok",
		"count":    count,
		"rejected": rejected,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// isNavSubmit 是否为导航式提交（GET 链接或 POST 表单）。
// 这类请求走浏览器主框架导航，不受 Chrome/Edge 的 LNA 策略拦截。
func isNavSubmit(r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	ct := r.Header.Get("Content-Type")
	return strings.HasPrefix(ct, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(ct, "multipart/form-data")
}

// parseAPIPayloads 解析推送请求，支持三种格式：
//  1. GET 查询参数：?url=<地址>&name=<名称>，url 可重复实现批量
//  2. 表单：url=<地址>&name=<名称>，url 可重复（application/x-www-form-urlencoded / multipart/form-data）
//  3. JSON：单个 {name,url} 或批量 [{name,url},...]（application/json）
//
// GET/表单方式的意义：新版 Chrome/Edge 的 LNA 策略会拦截「网页 → 127.0.0.1」的
// fetch/XHR，但主框架导航（地址栏打开链接、表单跳转、window.location）不受该限制，
// 因此可作为零配置、无密钥依赖的推送通道。
func parseAPIPayloads(r *http.Request) ([]apiPayload, error) {
	ct := r.Header.Get("Content-Type")

	// GET：从查询参数读取，url 可重复，name 按位置对应
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		urls := q["url"]
		if len(urls) == 0 {
			return nil, errors.New("missing url field")
		}
		names := q["name"]
		batch := make([]apiPayload, 0, len(urls))
		for i, u := range urls {
			p := apiPayload{URL: u}
			if i < len(names) {
				p.Name = names[i]
			}
			batch = append(batch, p)
		}
		return batch, nil
	}

	// 表单：url 可重复（批量），name 按位置一一对应，缺省留空
	if strings.HasPrefix(ct, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(maxAPIBody); err != nil && err != http.ErrNotMultipart {
			return nil, errors.New("parse form failed")
		}
		urls := r.PostForm["url"]
		if len(urls) == 0 {
			return nil, errors.New("missing url field")
		}
		names := r.PostForm["name"]
		batch := make([]apiPayload, 0, len(urls))
		for i, u := range urls {
			p := apiPayload{URL: u}
			if i < len(names) {
				p.Name = names[i]
			}
			batch = append(batch, p)
		}
		return batch, nil
	}

	// JSON：先尝试批量（数组），失败则回退为单个对象
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, errors.New("read body failed")
	}
	var batch []apiPayload
	if err := json.Unmarshal(body, &batch); err != nil {
		var one apiPayload
		if err := json.Unmarshal(body, &one); err != nil {
			return nil, errors.New("invalid json, expect {name,url} or [{name,url},...]")
		}
		batch = []apiPayload{one}
	}
	return batch, nil
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
