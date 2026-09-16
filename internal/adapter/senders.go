package adapter

import (
	"context"
	"net/url"
	"strings"
	"time"
)

func init() {
	RegisterSender("gotify", Gotify{})
	RegisterSender("ntfy", Ntfy{})
	RegisterSender("bark", Bark{})
	RegisterSender("telegram", Telegram{})
	RegisterSender("webhook", Webhook{})
	RegisterSender("custom", Webhook{}) // 自定义 HTTP 模板,与通用 webhook 同实现
}

// Gotify 出站:POST {url}/message?token=
type Gotify struct{}

func (Gotify) Send(ctx context.Context, cfg map[string]any, msg *Message) error {
	base := strings.TrimRight(CfgStr(cfg, "url"), "/")
	token := CfgStr(cfg, "token")
	if base == "" || token == "" {
		return ErrEmpty("url / token")
	}
	pri := msg.Priority
	if pri < 0 {
		pri = 0
	} else if pri > 10 {
		pri = 10
	}
	payload := map[string]any{"title": msg.Title, "message": msg.Body, "priority": pri}
	_, err := PostJSON(ctx, base+"/message?token="+url.QueryEscape(token), nil, payload)
	return err
}

// Ntfy 出站:POST {url}/{topic},优先级映射到 ntfy 的 1-5
type Ntfy struct{}

func (Ntfy) Send(ctx context.Context, cfg map[string]any, msg *Message) error {
	base := strings.TrimRight(CfgStr(cfg, "url"), "/")
	topic := CfgStr(cfg, "topic")
	if base == "" || topic == "" {
		return ErrEmpty("url / topic")
	}
	payload := map[string]any{"topic": topic, "title": msg.Title, "message": msg.Body}
	if len(msg.Tags) > 0 {
		payload["tags"] = msg.Tags
	}
	if msg.ClickURL != "" {
		payload["click"] = msg.ClickURL
	}
	if p := toNtfyPriority(msg.Priority); p > 0 {
		payload["priority"] = p
	}
	header := map[string]string{}
	if tok := CfgStr(cfg, "token"); tok != "" {
		header["Authorization"] = "Bearer " + tok
	} else if u := CfgStr(cfg, "username"); u != "" {
		header["Authorization"] = basicAuth(u, CfgStr(cfg, "password"))
	}
	_, err := PostJSON(ctx, base+"/"+topic, header, payload)
	return err
}

func toNtfyPriority(p int) int {
	switch {
	case p <= -2:
		return 1 // min
	case p == -1:
		return 2 // low
	case p == 0, p == 1, p == 2, p == 3:
		return 3 // default
	case p >= 4:
		return 5 // urgent
	}
	return 3
}

// Bark 出站:POST {url}/push
type Bark struct{}

func (Bark) Send(ctx context.Context, cfg map[string]any, msg *Message) error {
	base := strings.TrimRight(CfgStr(cfg, "url"), "/")
	key := CfgStr(cfg, "device_key")
	if base == "" || key == "" {
		return ErrEmpty("url / device_key")
	}
	payload := map[string]any{"device_key": key, "title": msg.Title, "body": msg.Body}
	if s := CfgStr(cfg, "sound"); s != "" {
		payload["sound"] = s
	}
	if s := CfgStr(cfg, "group"); s != "" {
		payload["group"] = s
	}
	switch {
	case msg.Priority >= 8:
		payload["level"] = "timeSensitive"
	case msg.Priority <= 0:
		payload["level"] = "passive"
	}
	if msg.ClickURL != "" {
		payload["url"] = msg.ClickURL
	}
	_, err := PostJSON(ctx, base+"/push", nil, payload)
	return err
}

// Telegram 出站:Bot API sendMessage
type Telegram struct{}

func (Telegram) Send(ctx context.Context, cfg map[string]any, msg *Message) error {
	token := CfgStr(cfg, "bot_token")
	chatID := CfgAnyStr(cfg, "chat_id")
	if token == "" || chatID == "" {
		return ErrEmpty("bot_token / chat_id")
	}
	text := msg.Body
	if msg.Title != "" {
		text = msg.Title + "\n" + msg.Body
	}
	payload := map[string]any{"chat_id": chatID, "text": text}
	if msg.Priority < 0 {
		payload["disable_notification"] = true
	}
	_, err := PostJSON(ctx, "https://api.telegram.org/bot"+token+"/sendMessage", nil, payload)
	return err
}

// Webhook 通用/自定义出站:有模板按模板渲染,否则发送标准 JSON;可自定义方法与请求头
type Webhook struct{}

func (Webhook) Send(ctx context.Context, cfg map[string]any, msg *Message) error {
	target := CfgStr(cfg, "url")
	if target == "" {
		return ErrEmpty("url")
	}
	method := strings.ToUpper(CfgStr(cfg, "method"))
	if method == "" {
		method = "POST"
	}
	header := ParseHeaders(cfg["headers"])
	if tpl := CfgStr(cfg, "body_template"); tpl != "" {
		body := RenderTemplate(tpl, msg)
		if _, ok := header["Content-Type"]; !ok {
			header["Content-Type"] = "application/json"
		}
		_, err := HTTPDo(ctx, method, target, header, "", []byte(body))
		return err
	}
	payload := map[string]any{
		"title":    msg.Title,
		"body":     msg.Body,
		"priority": msg.Priority,
		"tags":     msg.Tags,
		"time":     msg.Time.Format(time.RFC3339),
	}
	if msg.ClickURL != "" {
		payload["click"] = msg.ClickURL
	}
	_, err := PostJSON(ctx, target, header, payload)
	return err
}
