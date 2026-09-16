// Package inbound 入站协议解析:把 /i/{code} 收到的各协议请求解析为标准化消息并落库。
package inbound

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"massagegate/internal/adapter"
	"massagegate/internal/store"
)

var codeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

func ValidCode(code string) bool { return codeRe.MatchString(code) }

func Register(r *gin.Engine, db *gorm.DB) {
	h := &handler{db: db}
	r.Any("/i/*path", h.handle)
}

type handler struct{ db *gorm.DB }

// handle 统一入口:/i/{code} 与 /i/{code}/{任意兼容路径}
func (h *handler) handle(c *gin.Context) {
	code := strings.Trim(c.Param("path"), "/")
	if i := strings.Index(code, "/"); i >= 0 {
		code = code[:i]
	}
	var src store.Source
	if err := h.db.Where("code = ?", code).First(&src).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown source"})
		return
	}
	if !src.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "source disabled"})
		return
	}
	cfg := adapter.ParseConfig(src.Config)
	if tok := adapter.CfgStr(cfg, "token"); tok != "" {
		if c.Query("token") != tok && c.GetHeader("X-Token") != tok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
	}
	body, _ := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	raw := string(body)

	msg, perr := h.parse(&src, cfg, c, body)
	if perr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": perr.Error()})
		return
	}
	m := store.Message{
		SourceID:   src.ID,
		Title:      truncate(msg.Title, 500),
		Body:       truncate(msg.Body, 32<<10),
		Priority:   msg.Priority,
		Tags:       strings.Join(msg.Tags, ","),
		ClickURL:   msg.ClickURL,
		Raw:        truncate(raw, 64<<10),
		ReceivedAt: time.Now(),
	}
	if err := h.db.Create(&m).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	created := h.createDeliveries(&src, &m)
	c.JSON(http.StatusOK, gin.H{
		"id":     m.ID,
		"msg":    "ok",
		"routes": created,
	})
}

// createDeliveries 按命中的路由创建投递任务,返回命中的路由名
func (h *handler) createDeliveries(src *store.Source, m *store.Message) []string {
	var routes []store.Route
	h.db.Where("source_id = ? AND enabled = ?", src.ID, true).Find(&routes)
	hit := []string{}
	for _, rt := range routes {
		if rt.FilterKeyword != "" &&
			!strings.Contains(m.Title+m.Body, rt.FilterKeyword) {
			continue
		}
		d := store.Delivery{
			MessageID: m.ID,
			RouteID:   rt.ID,
			Status:    store.DeliveryPending,
			NextAt:    time.Now(),
		}
		if err := h.db.Create(&d).Error; err == nil {
			hit = append(hit, rt.Name)
		}
	}
	return hit
}

func (h *handler) parse(src *store.Source, cfg map[string]any, c *gin.Context, body []byte) (*adapter.Message, error) {
	switch src.Type {
	case "gotify":
		return parseGotify(body), nil
	case "ntfy":
		return parseNtfy(c, body), nil
	case "bark":
		return parseBark(c, body), nil
	case "webhook":
		return parseWebhook(cfg, body), nil
	case "telegram":
		return parseTelegram(body), nil
	case "wecom":
		return parseWeCom(cfg, body), nil
	case "dingtalk":
		return parseDingTalk(cfg, body), nil
	default:
		return nil, fmt.Errorf("不支持的入站类型: %s", src.Type)
	}
}

// ---- gotify:POST JSON {title, message, priority, extras} ----

type gotifyReq struct {
	Title    string         `json:"title"`
	Message  string         `json:"message"`
	Priority int            `json:"priority"`
	Extras   map[string]any `json:"extras"`
}

func parseGotify(body []byte) *adapter.Message {
	var r gotifyReq
	_ = json.Unmarshal(body, &r)
	msg := &adapter.Message{Title: r.Title, Body: r.Message, Priority: r.Priority}
	if v, ok := r.Extras["client::notification"].(map[string]any); ok {
		if s, ok := v["click"].(string); ok {
			msg.ClickURL = s
		}
	}
	return msg
}

// ---- ntfy:JSON 发布 或 Header 发布(消息体为纯文本) ----

func parseNtfy(c *gin.Context, body []byte) *adapter.Message {
	msg := &adapter.Message{}
	var j struct {
		Title    string   `json:"title"`
		Message  string   `json:"message"`
		Priority any      `json:"priority"`
		Tags     []string `json:"tags"`
		Click    string   `json:"click"`
	}
	if json.Unmarshal(body, &j) == nil && (j.Message != "" || j.Title != "" || j.Click != "") {
		msg.Title = j.Title
		msg.Body = j.Message
		msg.Priority = anyToInt(j.Priority)
		msg.Tags = j.Tags
		msg.ClickURL = j.Click
		return msg
	}
	// header 模式
	msg.Body = string(body)
	msg.Title = firstHeader(c, "X-Title", "Title")
	msg.Priority = anyToInt(firstHeaderAny(c, "X-Priority", "Priority"))
	if tags := firstHeader(c, "X-Tags", "Tags"); tags != "" {
		msg.Tags = strings.Split(tags, ",")
	}
	msg.ClickURL = firstHeader(c, "X-Click", "Click")
	return msg
}

func firstHeader(c *gin.Context, names ...string) string {
	for _, n := range names {
		if v := c.GetHeader(n); v != "" {
			return v
		}
	}
	return ""
}

func firstHeaderAny(c *gin.Context, names ...string) any {
	v := firstHeader(c, names...)
	if v == "" {
		return nil
	}
	return v
}

// ---- bark:GET 路径式 /{title}/{subtitle}/{body} + 查询参数,或 POST JSON ----

func parseBark(c *gin.Context, body []byte) *adapter.Message {
	msg := &adapter.Message{}
	var j map[string]any
	if len(body) > 0 && json.Unmarshal(body, &j) == nil {
		msg.Title = jstr(j, "title")
		sub := jstr(j, "subtitle")
		msg.Body = jstr(j, "body")
		if msg.Body == "" {
			msg.Body = jstr(j, "message")
		}
		if sub != "" {
			msg.Title = joinSpace(msg.Title, sub)
		}
		if lvl := jstr(j, "level"); lvl != "" {
			msg.Priority = barkLevel(lvl)
		}
		msg.ClickURL = jstr(j, "url")
		if msg.ClickURL == "" {
			msg.ClickURL = jstr(j, "click")
		}
	} else {
		parts := strings.Split(strings.Trim(c.Param("path"), "/"), "/")
		// parts[0]=code,之后为 1~3 段:body / title,body / title,subtitle,body
		rest := parts[1:]
		switch len(rest) {
		case 1:
			msg.Body = rest[0]
		case 2:
			msg.Title, msg.Body = rest[0], rest[1]
		case 3:
			msg.Title, msg.Body = joinSpace(rest[0], rest[1]), rest[2]
		}
		if len(rest) > 3 { // 4 段:bark 完整格式 group/title/subtitle/body
			msg.Title = joinSpace(rest[1], rest[2])
			msg.Body = rest[3]
		}
	}
	if q := c.Query("title"); q != "" {
		msg.Title = q
	}
	if q := c.Query("body"); q != "" {
		msg.Body = q
	}
	if q := c.Query("url"); q != "" {
		msg.ClickURL = q
	}
	return msg
}

func barkLevel(l string) int {
	switch l {
	case "critical":
		return 10
	case "timeSensitive":
		return 9
	case "active":
		return 5
	case "passive":
		return 0
	}
	if n, err := strconv.Atoi(l); err == nil {
		return n
	}
	return 5
}

// ---- 通用 webhook:任意 JSON,可配置字段路径 ----

func parseWebhook(cfg map[string]any, body []byte) *adapter.Message {
	msg := &adapter.Message{}
	var j map[string]any
	if json.Unmarshal(body, &j) != nil {
		msg.Body = string(body)
		return msg
	}
	titleFields := cfgFields(cfg, "title_field", []string{"title", "subject"})
	bodyFields := cfgFields(cfg, "body_field", []string{"message", "body", "content", "text"})
	msg.Title = firstPath(j, titleFields)
	msg.Body = firstPath(j, bodyFields)
	if msg.Title == "" && msg.Body == "" {
		// 都取不到时退化为原始 JSON 文本,保证内容不丢
		msg.Body = string(body)
	}
	pf := adapter.CfgStr(cfg, "priority_field")
	if pf == "" {
		pf = "priority"
	}
	if v, ok := pathValue(j, pf); ok {
		msg.Priority = anyToInt(v)
	}
	return msg
}

func cfgFields(cfg map[string]any, key string, def []string) []string {
	s := adapter.CfgStr(cfg, key)
	if s == "" {
		return def
	}
	return strings.Split(s, ",")
}

func firstPath(j map[string]any, fields []string) string {
	for _, f := range fields {
		if v, ok := pathValue(j, strings.TrimSpace(f)); ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
			if v != nil {
				return fmt.Sprint(v)
			}
		}
	}
	return ""
}

// pathValue 支持 a.b.c 的点路径取值
func pathValue(j map[string]any, path string) (any, bool) {
	cur := any(j)
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[seg]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// ---- 公共小工具 ----

func anyToInt(v any) int {
	switch t := v.(type) {
	case nil:
		return 0
	case float64:
		return int(t)
	case string:
		if n, err := strconv.Atoi(t); err == nil {
			return n
		}
		return ntfyWordPriority(t)
	case bool:
		if t {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func ntfyWordPriority(s string) int {
	switch strings.ToLower(s) {
	case "min":
		return 1
	case "low":
		return 2
	case "high":
		return 4
	case "urgent", "max":
		return 5
	default:
		return 3
	}
}

func jstr(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func joinSpace(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + " " + b
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ---- Telegram Bot:Webhook 回调的 Update 结构 ----

type telegramUpdate struct {
	UpdateID int `json:"update_id"`
	Message  *struct {
		MessageID int `json:"message_id"`
		From      *struct {
			ID        int64  `json:"id"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
			Username  string `json:"username"`
		} `json:"from"`
		Chat *struct {
			ID        int64  `json:"id"`
			Type      string `json:"type"`
			Title     string `json:"title"`
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
	ChannelPost *struct {
		MessageID int    `json:"message_id"`
		Text      string `json:"text"`
		Chat      *struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"chat"`
	} `json:"channel_post"`
}

func parseTelegram(body []byte) *adapter.Message {
	var u telegramUpdate
	if err := json.Unmarshal(body, &u); err != nil {
		return &adapter.Message{Body: string(body)}
	}
	msg := &adapter.Message{}
	// 优先处理普通消息
	if u.Message != nil && u.Message.Text != "" {
		msg.Body = u.Message.Text
		if u.Message.From != nil {
			msg.Title = "来自 " + u.Message.From.Username
			if msg.Title == "来自 " {
				msg.Title = "来自 " + strings.TrimSpace(u.Message.From.FirstName + " " + u.Message.From.LastName)
			}
		}
		if u.Message.Chat != nil {
			// Chat ID 用于区分来源
			msg.Tags = []string{fmt.Sprintf("chat_%d", u.Message.Chat.ID)}
		}
		return msg
	}
	// 频道消息
	if u.ChannelPost != nil && u.ChannelPost.Text != "" {
		msg.Body = u.ChannelPost.Text
		if u.ChannelPost.Chat != nil && u.ChannelPost.Chat.Title != "" {
			msg.Title = u.ChannelPost.Chat.Title
		}
		return msg
	}
	// 无法解析时返回原始内容
	msg.Body = string(body)
	return msg
}

// ---- 企业微信:应用回调消息 ----

type wecomReq struct {
	ToUserName   string `xml:"ToUserName"`
	FromUserName string `xml:"FromUserName"`
	CreateTime   int64  `xml:"CreateTime"`
	MsgType      string `xml:"MsgType"`
	Content      string `xml:"Content"`
	MsgId        int64  `xml:"MsgId"`
	AgentID      int    `xml:"AgentID"`
	Event        string `xml:"Event"`
	ChangeType   string `xml:"ChangeType"`
}

func parseWeCom(cfg map[string]any, body []byte) *adapter.Message {
	msg := &adapter.Message{}
	// 企业微信回调可能是加密的 XML,这里先处理明文 XML
	// 去掉 XML 声明(如有)
	xmlBody := body
	if idx := bytes.Index(body, []byte("<xml>")); idx >= 0 {
		xmlBody = body[idx:]
	}
	var req wecomReq
	if err := xml.Unmarshal(xmlBody, &req); err != nil {
		// 非 XML,尝试 JSON
		var j map[string]any
		if json.Unmarshal(body, &j) == nil {
			msg.Body = firstPath(j, []string{"Content", "content", "text", "message"})
			msg.Title = firstPath(j, []string{"Title", "title", "FromUserName", "from"})
			return msg
		}
		msg.Body = string(body)
		return msg
	}
	switch req.MsgType {
	case "text":
		msg.Body = req.Content
		msg.Title = "来自企业微信"
		if req.FromUserName != "" {
			msg.Title = "来自 " + req.FromUserName
		}
	case "event":
		msg.Title = "企业微信事件: " + req.Event
		if req.ChangeType != "" {
			msg.Body = "变更类型: " + req.ChangeType
		}
	default:
		if req.Content != "" {
			msg.Body = req.Content
		} else {
			msg.Body = string(body)
		}
		msg.Title = "企业微信消息"
	}
	return msg
}

// ---- 钉钉:机器人 Outgoing 回调 ----

type dingtalkReq struct {
	Msgtype   string `json:"msgtype"`
	Text      *struct {
		Content string `json:"content"`
	} `json:"text"`
	MsgId     string `json:"msgId"`
	CreateAt  int64  `json:"createAt"`
	SenderNick string `json:"senderNick"`
	SenderId   string `json:"senderId"`
	SenderCorpId string `json:"senderCorpId"`
	SenderDingtalkId string `json:"senderDingtalkId"`
	ConversationId   string `json:"conversationId"`
	AtUsers   []struct {
		DingtalkId string `json:"dingtalkId"`
	} `json:"atUsers"`
	IsAdmin      bool `json:"isAdmin"`
	IsInAtGroup  bool `json:"isInAtGroup"`
	ConversationType string `json:"conversationType"`
}

func parseDingTalk(cfg map[string]any, body []byte) *adapter.Message {
	msg := &adapter.Message{}
	var req dingtalkReq
	if err := json.Unmarshal(body, &req); err != nil {
		// 尝试解析为通用 JSON
		var j map[string]any
		if json.Unmarshal(body, &j) == nil {
			msg.Body = firstPath(j, []string{"text.content", "content", "Content"})
			msg.Title = firstPath(j, []string{"senderNick", "SenderNick", "title", "Title"})
			return msg
		}
		msg.Body = string(body)
		return msg
	}
	msg.Title = "钉钉消息"
	if req.SenderNick != "" {
		msg.Title = "来自 " + req.SenderNick
	}
	if req.Text != nil && req.Text.Content != "" {
		msg.Body = req.Text.Content
	}
	if req.ConversationId != "" {
		msg.Tags = []string{req.ConversationId}
	}
	return msg
}
