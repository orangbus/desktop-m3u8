package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ---------------------------------------------------------------------------
// 版本检查与在线更新
//
// 启动时异步请求 GitHub Releases 最新版本，与内置版本号比对；有新版本时向前端
// 推送 update:available 事件（前端弹出强制更新提示）。用户可选择：
//   1. 在线更新：下载当前平台的安装包（带进度推送 update:progress），Windows /
//      Linux 自动解压并替换主程序（重启生效）；macOS 解压后在访达中定位新版本
//   2. 跳转 GitHub Releases 页面手动下载
// ---------------------------------------------------------------------------

const (
	appVersion    = "1.0.2"
	updateRepoAPI = "https://api.github.com/repos/orangbus/desktop-m3u8/releases/latest"
	updateRepoURL = "https://github.com/orangbus/desktop-m3u8/releases/latest"
)

// UpdateInfo 更新信息（事件推送 / CheckUpdate 返回值，字段名即前端 JSON 键）。
type UpdateInfo struct {
	HasUpdate   bool   `json:"hasUpdate"`
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	Notes       string `json:"notes"`
	HTMLURL     string `json:"htmlUrl"`
	AssetName   string `json:"assetName"`
	DownloadURL string `json:"downloadUrl"`
	AssetSize   int64  `json:"assetSize"`
	Platform    string `json:"platform"`
}

// UpdateResult 在线更新的执行结果。
//   installed：已替换主程序，重启应用后生效
//   saved：    更新包已下载并解压（macOS），Path 为解压后的应用位置
type UpdateResult struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

// ghRelease GitHub Releases API 返回结构（只取需要的字段）。
type ghRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// updHTTP 版本检查客户端；下载大文件单独用无总超时的客户端（靠读写超时与错误终止）。
var updHTTP = &http.Client{Timeout: 15 * time.Second}

// checkUpdate 请求 GitHub 最新 Release 并与内置版本比对。网络失败时静默返回
// hasUpdate=false，不打断正常使用。
func (a *App) checkUpdate() *UpdateInfo {
	info := &UpdateInfo{HasUpdate: false, Current: appVersion, Platform: goruntime.GOOS}
	req, err := http.NewRequest(http.MethodGet, updateRepoAPI, nil)
	if err != nil {
		return info
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "desk-m3u8-updater")
	resp, err := updHTTP.Do(req)
	if err != nil {
		return info
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return info
	}
	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return info
	}
	latest := strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
	info.Latest = latest
	info.Notes = rel.Body
	info.HTMLURL = rel.HTMLURL
	if asset := pickAsset(rel.Assets); asset != nil {
		info.AssetName = asset.Name
		info.DownloadURL = asset.BrowserDownloadURL
		info.AssetSize = asset.Size
	}
	info.HasUpdate = isNewerVersion(latest, appVersion)
	return info
}

// CheckUpdate 供前端手动触发一次版本检查。
func (a *App) CheckUpdate() *UpdateInfo {
	return a.checkUpdate()
}

// OpenReleasePage 用系统浏览器打开 GitHub Releases 页面。
func (a *App) OpenReleasePage() {
	if a.ctx == nil {
		return
	}
	runtime.BrowserOpenURL(a.ctx, updateRepoURL)
}

// pickAsset 按当前平台与架构挑选安装包。
func pickAsset(assets []ghAsset) *ghAsset {
	return pickAssetFor(assets, goruntime.GOOS, goruntime.GOARCH)
}

// pickAssetFor 优先精确匹配 <os>-<arch>，找不到时回退到 amd64 产物
// （当前发布流水线只产出 amd64 与 darwin 双架构）。
func pickAssetFor(assets []ghAsset, osName, arch string) *ghAsset {
	want := fmt.Sprintf("%s-%s", osName, arch)
	var fallback string
	switch osName {
	case "windows":
		fallback = "windows-amd64"
	case "darwin":
		fallback = "darwin-amd64"
	default:
		fallback = "linux-amd64"
	}
	isPkg := func(n string) bool {
		return strings.HasSuffix(n, ".zip") || strings.HasSuffix(n, ".tar.gz")
	}
	var fb *ghAsset
	for i := range assets {
		n := strings.ToLower(assets[i].Name)
		if !isPkg(n) {
			continue
		}
		if strings.Contains(n, want) {
			return &assets[i]
		}
		if fb == nil && strings.Contains(n, fallback) {
			fb = &assets[i]
		}
	}
	return fb
}

// isNewerVersion 语义化版本比较（v 前缀与 -rc 后缀均可容忍）。
func isNewerVersion(latest, current string) bool {
	l := parseVer(latest)
	c := parseVer(current)
	for i := 0; i < 3; i++ {
		if l[i] > c[i] {
			return true
		}
		if l[i] < c[i] {
			return false
		}
	}
	return false
}

func parseVer(s string) [3]int {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	s = strings.SplitN(s, "-", 2)[0]
	parts := strings.Split(s, ".")
	var v [3]int
	for i := 0; i < len(parts) && i < 3; i++ {
		n, _ := strconv.Atoi(strings.TrimSpace(parts[i]))
		v[i] = n
	}
	return v
}

// DownloadUpdate 在线更新：下载当前平台安装包（推送 update:progress 进度事件），
// 解压后 Windows/Linux 直接替换主程序（重启生效），macOS 在访达中定位新应用。
func (a *App) DownloadUpdate() (*UpdateResult, error) {
	info := a.checkUpdate()
	if info == nil || !info.HasUpdate {
		return nil, errors.New("当前已是最新版本")
	}
	if info.DownloadURL == "" {
		return nil, errors.New("未找到适用于当前平台的更新包，请前往 GitHub 手动下载")
	}
	if a.ctx == nil {
		return nil, errors.New("应用尚未就绪")
	}
	tmpDir, err := os.MkdirTemp("", "desk-m3u8-update")
	if err != nil {
		return nil, err
	}
	archivePath := filepath.Join(tmpDir, info.AssetName)
	if err := downloadFile(info.DownloadURL, archivePath, info.AssetSize, func(done, total int64) {
		percent := 0
		if total > 0 {
			percent = int(done * 100 / total)
		}
		runtime.EventsEmit(a.ctx, "update:progress", map[string]interface{}{
			"downloaded": done,
			"total":      total,
			"percent":    percent,
		})
	}); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("下载更新包失败: %w", err)
	}
	extracted, err := extractUpdate(archivePath, tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("解压更新包失败: %w", err)
	}
	switch goruntime.GOOS {
	case "windows", "linux":
		if err := replaceBinary(extracted); err != nil {
			os.RemoveAll(tmpDir)
			return nil, fmt.Errorf("替换主程序失败: %w", err)
		}
		os.RemoveAll(tmpDir)
		return &UpdateResult{Status: "installed"}, nil
	default: // darwin：解压出 .app 后在访达中定位，由用户拖回应用程序目录
		_ = exec.Command("open", "-R", extracted).Start()
		return &UpdateResult{Status: "saved", Path: extracted}, nil
	}
}

// RestartApp 重启应用：先安全退出下载与本地接口，再以更新后的主程序重新启动。
func (a *App) RestartApp() {
	a.Shutdown()
	exe, err := os.Executable()
	if err != nil {
		runtime.Quit(a.ctx)
		return
	}
	switch goruntime.GOOS {
	case "windows":
		_ = exec.Command("cmd", "/c", "start", "", exe).Start()
	case "darwin":
		// exe 位于 foo.app/Contents/MacOS/foo，向上取三级得到应用包
		bundle := exe
		for i := 0; i < 3; i++ {
			bundle = filepath.Dir(bundle)
		}
		if strings.HasSuffix(bundle, ".app") {
			_ = exec.Command("open", bundle).Start()
		} else {
			_ = exec.Command("open", exe).Start()
		}
	default:
		_ = exec.Command(exe).Start()
	}
	runtime.Quit(a.ctx)
}

// downloadFile 流式下载并按 200ms 节流回调进度。
func downloadFile(url, dst string, total int64, onProgress func(done, total int64)) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "desk-m3u8-updater")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回 %s", resp.Status)
	}
	if total <= 0 {
		total = resp.ContentLength
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, 64<<10)
	var done int64
	lastEmit := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			done += int64(n)
			if onProgress != nil && time.Since(lastEmit) >= 200*time.Millisecond {
				lastEmit = time.Now()
				onProgress(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if onProgress != nil {
		onProgress(done, total)
	}
	return nil
}

// extractUpdate 从下载的压缩包中解出可执行内容，返回可执行文件（或 macOS .app）路径。
func extractUpdate(archivePath, tmpDir string) (string, error) {
	extractDir := filepath.Join(tmpDir, "extracted")
	if strings.HasSuffix(strings.ToLower(archivePath), ".zip") {
		return extractZip(archivePath, extractDir)
	}
	if strings.HasSuffix(strings.ToLower(archivePath), ".tar.gz") {
		return extractTarGz(archivePath, extractDir)
	}
	return "", fmt.Errorf("不支持的更新包格式: %s", filepath.Base(archivePath))
}

func extractZip(archivePath, extractDir string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer r.Close()
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return "", err
	}
	var bestExe string
	var bestSize uint64
	for _, zf := range r.File {
		name := zf.Name
		if strings.HasSuffix(name, "/") { // 目录项（macOS .app 目录结构由文件项带路径创建）
			continue
		}
		dst := filepath.Join(extractDir, filepath.FromSlash(name))
		if !strings.HasPrefix(filepath.Clean(dst), filepath.Clean(extractDir)+string(os.PathSeparator)) &&
			filepath.Clean(dst) != filepath.Clean(extractDir) {
			return "", fmt.Errorf("压缩包包含非法路径: %s", name) // 防 Zip Slip
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		if err := extractFile(zf, dst); err != nil {
			return "", err
		}
		mode := zf.Mode()
		// Windows：取最大的 .exe；其他平台取可执行位文件中最大的
		if goruntime.GOOS == "windows" {
			if strings.HasSuffix(strings.ToLower(name), ".exe") && zf.UncompressedSize64 > bestSize {
				bestExe, bestSize = dst, zf.UncompressedSize64
			}
		} else if mode&0o111 != 0 && zf.UncompressedSize64 > bestSize {
			bestExe, bestSize = dst, zf.UncompressedSize64
		}
	}
	if bestExe == "" {
		return "", errors.New("更新包中未找到可执行文件")
	}
	return bestExe, nil
}

func extractFile(zf *zip.File, dst string) error {
	src, err := zf.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, zf.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, src)
	return err
}

func extractTarGz(archivePath, extractDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return "", err
	}
	tr := tar.NewReader(gz)
	var bestExe string
	var bestSize int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		dst := filepath.Join(extractDir, filepath.FromSlash(hdr.Name))
		if !strings.HasPrefix(filepath.Clean(dst), filepath.Clean(extractDir)+string(os.PathSeparator)) &&
			filepath.Clean(dst) != filepath.Clean(extractDir) {
			return "", fmt.Errorf("压缩包包含非法路径: %s", hdr.Name)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		mode := os.FileMode(hdr.Mode)
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			return "", err
		}
		_, cpErr := io.Copy(out, tr)
		out.Close()
		if cpErr != nil {
			return "", cpErr
		}
		if mode&0o111 != 0 && hdr.Size > bestSize {
			bestExe, bestSize = dst, hdr.Size
		}
	}
	if bestExe == "" {
		return "", errors.New("更新包中未找到可执行文件")
	}
	return bestExe, nil
}

// replaceBinary 用新程序替换正在运行的主程序：
// 先把运行中的 exe 改名为 .old（Windows 允许改名运行中的程序），再把新程序移入原位。
// 失败时回滚。上一次更新遗留的 .old 在启动时清理。
func replaceBinary(newPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if filepath.Clean(exe) == filepath.Clean(newPath) {
		return nil
	}
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := moveFile(newPath, exe); err != nil {
		_ = os.Rename(old, exe) // 回滚，保持旧版本可用
		return err
	}
	return nil
}

// moveFile 移动文件：优先 rename，跨盘等场景回退为复制。目标为可执行文件时补执行权限。
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	out.Close()
	if err != nil {
		return err
	}
	if goruntime.GOOS != "windows" {
		_ = os.Chmod(dst, 0o755)
	}
	return os.Remove(src)
}

// cleanupOldBinary 启动时清理上次在线更新遗留的旧主程序备份。
func cleanupOldBinary() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	_ = os.Remove(exe + ".old")
}
