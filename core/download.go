package core

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// aesKeyLen AES-128 key 长度
const aesKeyLen = 16

// parseAndFetch 解析 m3u8 并下载 AES key。headers 为请求头。
func parseAndFetch(ctx context.Context, m3u8URL string, headers map[string]string) (*ParseResult, error) {
	client := NewHTTPClient(headers)
	body, err := client.Get(ctx, m3u8URL)
	if err != nil {
		return nil, fmt.Errorf("获取 m3u8 失败: %w", err)
	}
	text, err := io.ReadAll(body)
	body.Close()
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(m3u8URL)
	if err != nil {
		return nil, err
	}

	// 处理 Master playlist：选择分辨率最高（无分辨率按码率最高）的子流再解析
	if variants, ok := ParseMasterVariants(string(text)); ok {
		return parseAndFetch(ctx, ResolveURL(base, bestVariant(variants).URI), headers)
	}

	pl, err := ParseM3U8(string(text))
	if err != nil {
		return nil, fmt.Errorf("解析 m3u8 失败: %w", err)
	}

	res := &ParseResult{URL: base, Playlist: pl, KeysBytes: make(map[int][]byte)}
	for idx, key := range pl.Keys {
		if key.Method != CryptMethodAES {
			continue
		}
		keyURL := ResolveURL(base, key.URI)
		kb, err := client.GetBytes(ctx, keyURL)
		if err != nil {
			return nil, fmt.Errorf("获取解密 key 失败(%s): %w", keyURL, err)
		}
		if len(kb) > aesKeyLen {
			kb = kb[:aesKeyLen]
		}
		res.KeysBytes[idx] = kb
	}
	return res, nil
}

// downloadTask 执行单个任务的完整下载流程。返回输出文件路径。
func downloadTask(t *Task, onProgress func()) (string, error) {
	res, err := parseAndFetch(t.ctx, t.URL, t.Headers)
	if err != nil {
		return "", err
	}
	// 过滤分片：剔除地址中不包含 URL 标记值（index.m3u8 父目录名）的分片。
	// 例：URL https://.../20261011/5prtI9u6/index.m3u8 的标记为 5prtI9u6，
	//     只下载解析后地址里含 5prtI9u6 的分片。
	if token := extractToken(t.URL); token != "" {
		kept := make([]*Segment, 0, len(res.Playlist.Segments))
		for _, seg := range res.Playlist.Segments {
			segURL := ResolveURL(res.URL, seg.URI)
			if strings.Contains(segURL, token) {
				kept = append(kept, seg)
			}
		}
		// 若全部被剔除（通常意味着标记误判），回退保留全部分片，避免误删。
		if len(kept) > 0 {
			res.Playlist.Segments = kept
		}
	}
	t.playlist = res.Playlist
	t.keys = res.KeysBytes
	t.Segments = len(res.Playlist.Segments)
	t.setDoneSegs(0)

	if err := os.MkdirAll(t.dir, 0o755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	if err := t.downloadSegments(res, onProgress); err != nil {
		return "", err
	}

	// 切换到合并阶段：状态置为 merging 并通知前端（进度条不再回跳）。
	t.Status = StatusMerging
	if onProgress != nil {
		onProgress()
	}

	out, err := t.merge()
	if err != nil {
		return "", err
	}
	t.OutputPath = out
	return out, nil
}

// downloadSegments 并发下载全部分片到临时目录。已存在的分片跳过（断点续传）。
func (t *Task) downloadSegments(res *ParseResult, onProgress func()) error {
	segCount := len(res.Playlist.Segments)
	concurrency := t.concurrency()
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var firstErr atomic.Pointer[error]
	var done atomic.Int64

	for idx := 0; idx < segCount; idx++ {
		select {
		case <-t.ctx.Done():
			return context.Canceled
		default:
		}
		segPath := filepath.Join(t.dir, fmt.Sprintf("%d.ts", idx))
		// 已下载（文件存在且非空）则跳过——这就是断点续传的“已下分片记录”。
		// 重试时无需重头开始，只补下缺失分片。
		if fi, err := os.Stat(segPath); err == nil && fi.Size() > 0 {
			n := done.Add(1)
			t.setDoneSegs(int(n))
			if onProgress != nil {
				onProgress()
			}
			continue
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()

			select {
			case <-t.ctx.Done():
				return
			default:
			}
			seg := res.Playlist.Segments[i]
			segURL := ResolveURL(res.URL, seg.URI)
			body, err := t.client.Get(t.ctx, segURL)
			if err != nil {
				e := fmt.Errorf("分片 %d 下载失败: %w", i, err)
				firstErr.CompareAndSwap(nil, &e)
				return
			}
			data, err := io.ReadAll(body)
			body.Close()
			if err != nil {
				// 慢速网络下读体中断是常态，直接透传 ctx 取消
				firstErr.CompareAndSwap(nil, &err)
				return
			}
			if seg.KeyIndex >= 0 && t.keys != nil {
				if keyBytes, ok := t.keys[seg.KeyIndex]; ok {
					iv, ivErr := keyIVBytes(res.Playlist.Keys[seg.KeyIndex].IV, seg.Seq)
					if ivErr != nil {
						e := fmt.Errorf("分片 %d IV 无效: %w", i, ivErr)
						firstErr.CompareAndSwap(nil, &e)
						return
					}
					data, err = aesDecrypt(data, keyBytes, iv)
					if err != nil {
						e := fmt.Errorf("分片 %d 解密失败: %w", i, err)
						firstErr.CompareAndSwap(nil, &e)
						return
					}
				}
			}
			segPath := filepath.Join(t.dir, fmt.Sprintf("%d.ts", i))
			if err := os.WriteFile(segPath, data, 0o644); err != nil {
				firstErr.CompareAndSwap(nil, &err)
				return
			}
			n := done.Add(1)
			t.setDoneSegs(int(n))
			t.addBytes(int64(len(data)))
			if onProgress != nil {
				onProgress()
			}
		}(idx)
	}
	wg.Wait()
	// 若被取消，返回 context.Canceled
	select {
	case <-t.ctx.Done():
		return context.Canceled
	default:
	}
	if fe := firstErr.Load(); fe != nil {
		return *fe
	}
	return nil
}

// concurrency 返回单任务分片并发数。
func (t *Task) concurrency() int {
	if t.conc > 0 {
		return t.conc
	}
	return 10
}

// merge 按序合并所有 ts 分片为一个视频文件，输出到最终目录。
func (t *Task) merge() (string, error) {
	name := sanitizeFilename(t.Filename)
	if name == "" {
		name = "video"
	}
	lower := strings.ToLower(name)
	if !strings.HasSuffix(lower, ".mp4") && !strings.HasSuffix(lower, ".ts") {
		name += ".mp4"
	}
	// 同名任务防覆盖：已存在则追加序号 video(1).mp4
	out := uniqueOutputPath(t.outDir, name)
	f, err := os.Create(out)
	if err != nil {
		return "", err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for i := 0; i < t.Segments; i++ {
		select {
		case <-t.ctx.Done():
			_ = os.Remove(out)
			return "", context.Canceled
		default:
		}
		segPath := filepath.Join(t.dir, fmt.Sprintf("%d.ts", i))
		// 分片缺失说明下载不完整，必须报错而不是静默产出残缺视频
		data, err := os.ReadFile(segPath)
		if err != nil {
			_ = os.Remove(out)
			return "", fmt.Errorf("分片 %d 缺失，无法合并: %w", i, err)
		}
		if _, err := w.Write(data); err != nil {
			// 合并中断：清理残留的半成品文件，避免删除任务时留下孤儿文件
			_ = os.Remove(out)
			return "", err
		}
	}
	if err := w.Flush(); err != nil {
		_ = os.Remove(out)
		return "", err
	}
	// 清理临时分片目录
	_ = os.RemoveAll(t.dir)
	return out, nil
}

// uniqueOutputPath 在 dir 下返回不冲突的输出路径；name 已存在时追加 (1)、(2)…
func uniqueOutputPath(dir, name string) string {
	p := filepath.Join(dir, name)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return p
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		p = filepath.Join(dir, fmt.Sprintf("%s(%d)%s", base, i, ext))
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p
		}
	}
}

// sanitizeFilename 去除文件名中的非法字符（Windows 不允许 <>:"/\|?*），
// 并处理 Windows 保留设备名（CON、PRN、COM1…）。
func sanitizeFilename(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	replacer := strings.NewReplacer("<", "", ">", "", ":", "", "\"", "", "/", "", "\\", "", "|", "", "?", "", "*", "", "\x00", "")
	name = replacer.Replace(name)
	// Windows 保留名（含带扩展名的情况，如 CON.mp4）：加前缀避开
	stem := strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))
	if winReservedNames[stem] {
		name = "_" + name
	}
	return name
}

// winReservedNames Windows 保留设备名（不区分大小写）。
var winReservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// aesDecrypt AES-128-CBC 解密。key 为 16 字节，iv 为 16 字节 IV。
func aesDecrypt(data, key []byte, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(iv) != aes.BlockSize {
		return nil, errors.New("invalid IV length")
	}
	if len(data)%aes.BlockSize != 0 {
		pad := aes.BlockSize - len(data)%aes.BlockSize
		data = append(data, bytes.Repeat([]byte{0}, pad)...)
	}
	dec := cipher.NewCBCDecrypter(block, iv)
	dec.CryptBlocks(data, data)
	if len(data) > 0 {
		padLen := int(data[len(data)-1])
		if padLen > 0 && padLen <= aes.BlockSize {
			data = data[:len(data)-padLen]
		}
	}
	return data, nil
}
