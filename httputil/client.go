// Package httputil 提供带上下文、状态检查、响应大小限制的 JSON 请求和流式下载。
package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/injoyai/util/oss"
)

// Client 的配置创建后不可修改。传入的 http.Client 可以定制代理及 Transport，
// 调用方不得在请求执行时修改它的配置。不会自动重试有副作用的请求。
type Client struct {
	client  *http.Client
	maxBody int64
}

// New nil client 使用 30 秒超时；maxBody=0 使用 4 MiB JSON 响应上限。
func New(client *http.Client, maxBody int64) (*Client, error) {
	if maxBody == 0 {
		maxBody = 4 << 20
	}
	if maxBody < 0 || maxBody == math.MaxInt64 {
		return nil, errors.New("httputil: invalid response limit")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{client: client, maxBody: maxBody}, nil
}

// StatusError 支持 errors.As 获取非 2xx 响应状态；不在错误中包含响应正文。
type StatusError struct {
	StatusCode int
	Status     string
}

func (e *StatusError) Error() string { return "httputil: HTTP " + e.Status }

func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, &StatusError{resp.StatusCode, resp.Status}
	}
	return resp, nil
}

// JSON 将非 nil body 编码为 JSON，非 nil dst 解码响应，204/205 无正文时跳过。
// headers 会克隆。解析错误、超限和非 2xx 均返回错误。始终关闭响应正文。
func (c *Client) JSON(ctx context.Context, method, url string, body, dst any, headers http.Header) error {
	var input io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		input = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, input)
	if err != nil {
		return err
	}
	req.Header = headers.Clone()
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		return err
	}
	if int64(len(b)) > c.maxBody {
		return errors.New("httputil: response body limit exceeded")
	}
	if dst == nil || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusResetContent {
		return nil
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("httputil: decode JSON: %w", err)
	}
	return nil
}

// Download GET 到文件，maxBytes 必须>0；失败保留已有目标并删除临时文件。
// 返回实际读取字节数（失败时也可能非零）。限制作用于解码后的响应正文。
func (c *Client) Download(ctx context.Context, url, filename string, maxBytes int64) (int64, error) {
	if maxBytes <= 0 || maxBytes == math.MaxInt64 {
		return 0, errors.New("httputil: invalid download limit")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.ContentLength > maxBytes {
		return 0, errors.New("httputil: download limit exceeded")
	}
	var n int64
	err = oss.Write(filename, 0644, func(w io.Writer) error {
		var err error
		n, err = io.Copy(w, io.LimitReader(resp.Body, maxBytes+1))
		if err != nil {
			return err
		}
		if n > maxBytes {
			return errors.New("httputil: download limit exceeded")
		}
		return ctx.Err()
	})
	return n, err
}
