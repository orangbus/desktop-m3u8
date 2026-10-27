package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// reqTimeout 单个 HTTP 请求的硬超时。
// 覆盖连接 + 完整响应体读取；取 2 分钟以兼容慢速网络下载较大分片。
const reqTimeout = 120 * time.Second

// HTTPClient 支持自定义请求头的 HTTP 客户端，用于 m3u8 / ts / key 下载。
// 很多流媒体站点需要 Referer / User-Agent 等请求头才能正常返回数据。
type HTTPClient struct {
	client  *http.Client
	headers map[string]string
}

// NewHTTPClient 创建一个 HTTP 客户端，headers 为请求头键值对（如 User-Agent、Referer）。
func NewHTTPClient(headers map[string]string) *HTTPClient {
	return &HTTPClient{
		// Client.Timeout 覆盖连接到响应体读取完毕的整个周期，且自动清理；
		// 比 context.WithTimeout 更适合「返回 body 后再读」的场景。
		client:  &http.Client{Timeout: reqTimeout},
		headers: headers,
	}
}

// Get 发起 GET 请求，返回响应体。状态码非 200 时返回错误。
// 请求绑定 ctx：任务暂停/取消时进行中的请求会被立即中断。
func (c *HTTPClient) Get(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("http error: status %d for %s", resp.StatusCode, url)
	}
	return resp.Body, nil
}

// GetBytes 下载整个资源为字节切片。
func (c *HTTPClient) GetBytes(ctx context.Context, url string) ([]byte, error) {
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(body)
}
