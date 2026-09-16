package adapter

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func init() {
	RegisterSender("wecom", WeCom{})
	RegisterSender("feishu", Feishu{})
	RegisterSender("dingtalk", DingTalk{})
}

// joinTitleBody 标题与正文合成一条文本
func joinTitleBody(title, body string) string {
	switch {
	case title == "":
		return body
	case body == "":
		return title
	}
	return title + "\n" + body
}

// WeCom 企业微信群机器人:POST 完整 webhook 地址
type WeCom struct{}

func (WeCom) Send(ctx context.Context, cfg map[string]any, msg *Message) error {
	target := CfgStr(cfg, "url")
	if target == "" {
		return ErrEmpty("url(机器人 webhook 地址)")
	}
	var payload map[string]any
	if strings.EqualFold(CfgStr(cfg, "msg_type"), "markdown") {
		content := msg.Body
		if msg.Title != "" {
			content = "**" + msg.Title + "**\n" + msg.Body
		}
		payload = map[string]any{"msgtype": "markdown", "markdown": map[string]any{"content": content}}
	} else {
		payload = map[string]any{"msgtype": "text", "text": map[string]any{"content": joinTitleBody(msg.Title, msg.Body)}}
	}
	_, err := PostJSON(ctx, target, nil, payload)
	return err
}

// Feishu 飞书自定义机器人:支持签名校验(HMAC-SHA256,key = "timestamp\nsecret")
type Feishu struct{}

func (Feishu) Send(ctx context.Context, cfg map[string]any, msg *Message) error {
	target := CfgStr(cfg, "url")
	if target == "" {
		return ErrEmpty("url(机器人 webhook 地址)")
	}
	payload := map[string]any{
		"msg_type": "text",
		"content":  map[string]any{"text": joinTitleBody(msg.Title, msg.Body)},
	}
	if secret := CfgStr(cfg, "secret"); secret != "" {
		ts := time.Now().Unix()
		mac := hmac.New(sha256.New, []byte(fmt.Sprintf("%d\n%s", ts, secret)))
		payload["timestamp"] = fmt.Sprint(ts)
		payload["sign"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	}
	_, err := PostJSON(ctx, target, nil, payload)
	return err
}

// DingTalk 钉钉自定义机器人:支持加签(HMAC-SHA256,签名拼到 URL 查询串)
type DingTalk struct{}

func (DingTalk) Send(ctx context.Context, cfg map[string]any, msg *Message) error {
	target := CfgStr(cfg, "url")
	if target == "" {
		return ErrEmpty("url(机器人 webhook 地址)")
	}
	if secret := CfgStr(cfg, "secret"); secret != "" {
		ts := time.Now().UnixMilli()
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(fmt.Sprintf("%d\n%s", ts, secret)))
		sign := url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		target = fmt.Sprintf("%s%stimestamp=%d&sign=%s", target, sep, ts, sign)
	}
	var payload map[string]any
	if strings.EqualFold(CfgStr(cfg, "msg_type"), "markdown") {
		payload = map[string]any{"msgtype": "markdown", "markdown": map[string]any{"title": msg.Title, "text": joinTitleBody(msg.Title, msg.Body)}}
	} else {
		payload = map[string]any{"msgtype": "text", "text": map[string]any{"content": joinTitleBody(msg.Title, msg.Body)}}
	}
	_, err := PostJSON(ctx, target, nil, payload)
	return err
}
