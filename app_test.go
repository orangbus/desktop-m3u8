package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestCORSHeaders 验证本地接口对浏览器跨域/私有网络访问预检的响应头完整。
// 从网页（尤其 https 页面）向 127.0.0.1 提交时，Chrome/Edge 会视为跨地址空间
// 请求，预检响应必须带 Access-Control-Allow-Private-Network，否则提交失败。
func TestCORSHeaders(t *testing.T) {
	handler := cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/download", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type,x-api-key")
	req.Header.Set("Access-Control-Request-Private-Network", "true")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	checks := map[string]string{
		"Access-Control-Allow-Origin":          "https://example.com",
		"Access-Control-Allow-Methods":         "POST, OPTIONS",
		"Access-Control-Allow-Headers":         "Content-Type, X-Api-Key, Authorization",
		"Access-Control-Allow-Private-Network": "true",
	}
	for header, want := range checks {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

// TestCORSHeadersNoOrigin 无 Origin 的请求（curl/Postman）应回退为通配 *。
func TestCORSHeadersNoOrigin(t *testing.T) {
	handler := cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodPost, "/api/download", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
	}
}

// TestParseAPIPayloadsForm 验证表单解析：单个、批量、名称对应、缺 url 报错。
func TestParseAPIPayloadsForm(t *testing.T) {
	cases := []struct {
		name     string
		form     url.Values
		wantLen  int
		wantName string
		wantErr  bool
	}{
		{"单个", url.Values{"url": {"https://a.m3u8"}, "name": {"视频A"}}, 1, "视频A", false},
		{"批量带名称", url.Values{"url": {"https://a.m3u8", "https://b.m3u8"}, "name": {"视频A", "视频B"}}, 2, "视频A", false},
		{"批量无名称", url.Values{"url": {"https://a.m3u8", "https://b.m3u8"}}, 2, "", false},
		{"缺少url", url.Values{"name": {"只有名字"}}, 0, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.form.Encode()
			req := httptest.NewRequest(http.MethodPost, "/api/download", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			got, err := parseAPIPayloads(req)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if len(got) != tc.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tc.wantLen)
			}
			if tc.wantName != "" && got[0].Name != tc.wantName {
				t.Errorf("name = %q, want %q", got[0].Name, tc.wantName)
			}
		})
	}
}

// TestParseAPIPayloadsGET 验证 GET 查询参数解析：支持批量与 name 对应。
func TestParseAPIPayloadsGET(t *testing.T) {
	cases := []struct {
		name     string
		target   string
		wantLen  int
		wantName string
		wantErr  bool
	}{
		{"单个", "/api/download?url=https%3A%2F%2Fa.m3u8&name=%E8%A7%86%E9%A2%91A", 1, "视频A", false},
		{"批量", "/api/download?url=https%3A%2F%2Fa.m3u8&url=https%3A%2F%2Fb.m3u8&name=A&name=B", 2, "A", false},
		{"缺少url", "/api/download?name=A", 0, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			got, err := parseAPIPayloads(req)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if len(got) != tc.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tc.wantLen)
			}
			if tc.wantName != "" && got[0].Name != tc.wantName {
				t.Errorf("name = %q, want %q", got[0].Name, tc.wantName)
			}
		})
	}
}

func TestIsNavSubmit(t *testing.T) {
	cases := []struct {
		method string
		ct     string
		want   bool
	}{
		{http.MethodGet, "", true},
		{http.MethodPost, "application/x-www-form-urlencoded", true},
		{http.MethodPost, "multipart/form-data; boundary=x", true},
		{http.MethodPost, "application/json", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, "/api/download", nil)
		if tc.ct != "" {
			req.Header.Set("Content-Type", tc.ct)
		}
		if got := isNavSubmit(req); got != tc.want {
			t.Errorf("method=%s ct=%s => %v, want %v", tc.method, tc.ct, got, tc.want)
		}
	}
}

// TestParseAPIPayloadsJSON 验证 JSON 单个/批量解析未回归。
func TestParseAPIPayloadsJSON(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantLen int
	}{
		{"单个", `{"name":"A","url":"https://a.m3u8"}`, 1},
		{"批量", `[{"name":"A","url":"https://a.m3u8"},{"name":"B","url":"https://b.m3u8"}]`, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/download", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			got, err := parseAPIPayloads(req)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if len(got) != tc.wantLen {
				t.Errorf("len = %d, want %d", len(got), tc.wantLen)
			}
		})
	}
}
