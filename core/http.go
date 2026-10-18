package core

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient 支持自定义请求头的 HTTP 客户端，用于 m3u8 / ts / key 下载。
// 很多流媒体站点需要 Referer / User-Agent 等请求头才能正常返回数据。
type HTTPClient struct {
	client  *http.Client
	headers map[string]string
}

// NewHTTPClient 创建一个 HTTP 客户端，headers 为请求头键值对（如 User-Agent、Referer）。
func NewHTTPClient(headers map[string]string) *HTTPClient {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}
	return &HTTPClient{client: client, headers: headers}
}

// Get 发起 GET 请求，返回响应体。状态码非 200 时返回错误。
func (c *HTTPClient) Get(url string) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
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
func (c *HTTPClient) GetBytes(url string) ([]byte, error) {
	body, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(body)
}
