package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 端到端回归：模拟站点返回「无 IV 的 AES-128 加密分片 + METHOD=NONE 轮换」的
// M3U8，完整走一遍 downloadTask（解析 → 下载 → 解密 → 合并），
// 验证产物内容逐字节正确。
func TestDownloadTaskEndToEnd(t *testing.T) {
	key := []byte("0123456789abcdef")
	plain0 := []byte("SEGMENT-0-encrypted-with-seq-IV--")
	plain1 := []byte("SEGMENT-1-plain-after-none-key--")
	plain2 := []byte("SEGMENT-2-plain--")

	seg0 := encryptCBC(plain0, key, mustIV(t, "", 5)) // MEDIA-SEQUENCE:5，无 IV

	mux := http.NewServeMux()
	mux.HandleFunc("/video/index.m3u8", func(w http.ResponseWriter, r *http.Request) {
		// 分片 0 加密（无 IV，靠序列号 5），分片 1/2 明文（METHOD=NONE）
		_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:10\n#EXT-X-MEDIA-SEQUENCE:5\n" +
			"#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\n#EXTINF:10,\nseg0.ts\n" +
			"#EXT-X-KEY:METHOD=NONE\n#EXTINF:10,\nseg1.ts\n#EXTINF:10,\nseg2.ts\n" +
			"#EXT-X-ENDLIST\n"))
	})
	mux.HandleFunc("/video/key.bin", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(key)
	})
	mux.HandleFunc("/video/seg0.ts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(seg0)
	})
	mux.HandleFunc("/video/seg1.ts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(plain1)
	})
	mux.HandleFunc("/video/seg2.ts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(plain2)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	outDir := t.TempDir()
	m := NewManager(DefaultSettings(), nil)
	t.Cleanup(m.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	task := &Task{
		ID:       "e2e-test",
		URL:      srv.URL + "/video/index.m3u8",
		Filename: "e2e.mp4",
		Status:   StatusWaiting,
		dir:      filepath.Join(outDir, ".tmp", "e2e-test"),
		outDir:   outDir,
		client:   NewHTTPClient(nil),
		conc:     2,
		ctx:      ctx,
		cancel:   cancel,
	}

	sawMerging := false
	out, err := downloadTask(task, func() {
		if task.Status == StatusMerging {
			sawMerging = true
		}
	})
	if err != nil {
		t.Fatalf("downloadTask failed: %v", err)
	}
	if !sawMerging {
		t.Error("expected merging status to be reported during merge phase")
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	want := string(plain0) + string(plain1) + string(plain2)
	if string(got) != want {
		t.Fatalf("merged content mismatch:\n got %q\nwant %q", got, want)
	}
	// 合并完成后临时分片目录应被清理
	if _, err := os.Stat(task.dir); !os.IsNotExist(err) {
		t.Error("tmp segment dir should be removed after merge")
	}
	// 断点续传：模拟"上次合并失败"的真实场景（成品不存在、部分分片已落盘），
	// 重试只补缺失分片且产物仍然正确
	if err := os.Remove(out); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(task.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(task.dir, "1.ts"), plain1, 0o644); err != nil {
		t.Fatal(err)
	}
	out2, err := downloadTask(task, nil)
	if err != nil {
		t.Fatalf("resume download failed: %v", err)
	}
	if out2 != out {
		t.Fatalf("resume output = %q, want %q", out2, out)
	}
	got2, _ := os.ReadFile(out2)
	if string(got2) != want {
		t.Fatalf("resume merged content mismatch")
	}
	// 临时目录应再次被清理
	if _, err := os.Stat(task.dir); !os.IsNotExist(err) {
		t.Error("tmp dir should be removed after resumed merge")
	}
}

// 分片缺失时 merge 必须报错而不是产出残缺视频
func TestMergeFailsOnMissingSegment(t *testing.T) {
	outDir := t.TempDir()
	dir := filepath.Join(outDir, ".tmp", "missing-seg")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 只写了分片 0，分片 1 缺失
	if err := os.WriteFile(filepath.Join(dir, "0.ts"), []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tk := &Task{
		ID: "missing-seg", Filename: "x.mp4", Segments: 2,
		dir: dir, outDir: outDir, ctx: ctx, cancel: cancel,
	}
	_, err := tk.merge()
	if err == nil || !strings.Contains(err.Error(), "缺失") {
		t.Fatalf("merge should fail on missing segment, got err=%v", err)
	}
}

func mustIV(t *testing.T, ivHex string, seq int64) []byte {
	t.Helper()
	iv, err := keyIVBytes(ivHex, seq)
	if err != nil {
		t.Fatal(err)
	}
	return iv
}
