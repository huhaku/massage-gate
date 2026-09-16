package adapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var httpClient = &http.Client{} // 超时由调用方通过 ctx deadline 控制

// HTTPDo 发起 HTTP 请求;ctx 无 deadline 时兜底 30s
func HTTPDo(ctx context.Context, method, url string, header map[string]string, contentType string, body []byte) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "massage-gate/1.0")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode >= 300 {
		return data, fmt.Errorf("HTTP %d: %s", resp.StatusCode, Snippet(data, 200))
	}
	return data, nil
}

// PostJSON 以 JSON 体 POST
func PostJSON(ctx context.Context, url string, header map[string]string, payload any) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return HTTPDo(ctx, "POST", url, header, "application/json", b)
}

func Snippet(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		s = s[:n] + "..."
	}
	return s
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// RenderTemplate 渲染自定义模板占位符;
// {{title}} {{body}} {{priority}} {{tags}} {{time}} 及 JSON 转义版 {{title_json}} {{body_json}}
func RenderTemplate(tpl string, msg *Message) string {
	ts := "2006-01-02 15:04:05"
	if !msg.Time.IsZero() {
		ts = msg.Time.Local().Format("2006-01-02 15:04:05")
	}
	return strings.NewReplacer(
		"{{title_json}}", jsonStr(msg.Title),
		"{{body_json}}", jsonStr(msg.Body),
		"{{title}}", msg.Title,
		"{{body}}", msg.Body,
		"{{priority}}", strconv.Itoa(msg.Priority),
		"{{tags}}", strings.Join(msg.Tags, ","),
		"{{time}}", ts,
	).Replace(tpl)
}

func basicAuth(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

// ParseHeaders 解析配置中的 headers(对象或 JSON 字符串均可)
func ParseHeaders(v any) map[string]string {
	h := map[string]string{}
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			h[k] = fmt.Sprint(vv)
		}
	case string:
		if t != "" {
			m := map[string]any{}
			if json.Unmarshal([]byte(t), &m) == nil {
				for k, vv := range m {
					h[k] = fmt.Sprint(vv)
				}
			}
		}
	}
	return h
}
