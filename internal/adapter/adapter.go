// Package adapter 出站适配器:把标准化消息推送到各协议的消息服务。
// 新协议只需实现 Sender 并调用 RegisterSender 即可接入。
package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Message 标准化消息
type Message struct {
	Title    string
	Body     string
	Priority int
	Tags     []string
	ClickURL string
	Time     time.Time
}

// Sender 出站适配器接口
type Sender interface {
	Send(ctx context.Context, cfg map[string]any, msg *Message) error
}

var senders = map[string]Sender{}

func RegisterSender(name string, s Sender) { senders[name] = s }

func HasSender(name string) bool {
	_, ok := senders[name]
	return ok
}

// SenderTypes 已注册的出站类型名(排序后)
func SenderTypes() []string {
	names := make([]string, 0, len(senders))
	for n := range senders {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Send 按类型调用对应适配器发送
func Send(ctx context.Context, typ string, cfg map[string]any, msg *Message) error {
	s, ok := senders[typ]
	if !ok {
		return fmt.Errorf("未知出站类型: %s", typ)
	}
	return s.Send(ctx, cfg, msg)
}

// ParseConfig 解析存储在通道配置列里的 JSON
func ParseConfig(s string) map[string]any {
	m := map[string]any{}
	if s != "" {
		_ = json.Unmarshal([]byte(s), &m)
	}
	return m
}

func CfgStr(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return strings.TrimSpace(v)
}

func CfgAnyStr(m map[string]any, k string) string {
	if s, ok := m[k].(string); ok {
		return strings.TrimSpace(s)
	}
	return fmt.Sprint(m[k])
}

func CfgInt(m map[string]any, k string) int {
	switch v := m[k].(type) {
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}

func ErrEmpty(field string) error {
	return errors.New("缺少必填配置: " + field)
}
