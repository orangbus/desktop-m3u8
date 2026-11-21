package core

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestMaxConcurrentTasksEnforced 实测：即使一次性加入大量任务，同时处于
// running（parsing/downloading/merging）的任务数也不得超过 MaxConcurrentTasks。
func TestMaxConcurrentTasksEnforced(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	const maxTasks = 2
	slowSeg := func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(120 * time.Millisecond)
		_, _ = w.Write([]byte("segment-data-0123456789"))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v/index.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:10\n#EXTINF:10,\n0.ts\n#EXTINF:10,\n1.ts\n#EXTINF:10,\n2.ts\n#EXT-X-ENDLIST\n"))
	})
	mux.HandleFunc("/v/0.ts", slowSeg)
	mux.HandleFunc("/v/1.ts", slowSeg)
	mux.HandleFunc("/v/2.ts", slowSeg)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	s := DefaultSettings()
	s.MaxConcurrentTasks = maxTasks
	s.ConcurrencyPerTask = 2
	s.SavePath = dir
	m := NewManager(s, nil)
	defer m.Close()

	ids := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		ids = append(ids, m.AddTask(fmt.Sprintf("%s/v/index.m3u8", srv.URL), fmt.Sprintf("v%d", i), nil).ID)
	}

	var peak int64
	stop := make(chan struct{})
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		for {
			select {
			case <-stop:
				return
			default:
			}
			m.mu.Lock()
			r := int64(m.running)
			m.mu.Unlock()
			for {
				old := atomic.LoadInt64(&peak)
				if r <= old || atomic.CompareAndSwapInt64(&peak, old, r) {
					break
				}
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		allDone := true
		for _, id := range ids {
			tk := m.tasks[id]
			if tk == nil || (tk.Status != StatusDone && tk.Status != StatusFailed) {
				allDone = false
				break
			}
		}
		m.mu.Unlock()
		if allDone {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	close(stop)
	<-sampled

	got := atomic.LoadInt64(&peak)
	if got > int64(maxTasks) {
		t.Fatalf("peak concurrent = %d, want <= %d", got, maxTasks)
	}
	if got < 1 {
		t.Fatalf("peak concurrent = %d, expected some concurrency", got)
	}
	t.Logf("peak concurrent = %d (max %d) OK", got, maxTasks)
}
