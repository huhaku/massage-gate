// Package api 管理后台 REST API(单用户,会话 Cookie 认证)
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"massagegate/internal/adapter"
	"massagegate/internal/dispatch"
	"massagegate/internal/inbound"
	"massagegate/internal/store"
)

type API struct {
	db       *gorm.DB
	sessions sync.Map // token -> expiry
}

func Register(r *gin.Engine, db *gorm.DB) {
	a := &API{db: db}
	g := r.Group("/api")
	g.GET("/meta", a.meta)
	g.POST("/init", a.init)
	g.POST("/login", a.login)

	auth := g.Group("", a.requireAuth)
	auth.POST("/logout", a.logout)
	auth.GET("/me", a.me)

	auth.GET("/sources", a.listSources)
	auth.POST("/sources", a.saveSource)
	auth.PUT("/sources/:id", a.saveSource)
	auth.DELETE("/sources/:id", a.deleteSource)

	auth.GET("/targets", a.listTargets)
	auth.POST("/targets", a.saveTarget)
	auth.PUT("/targets/:id", a.saveTarget)
	auth.DELETE("/targets/:id", a.deleteTarget)
	auth.POST("/targets/:id/test", a.testTarget)

	auth.GET("/routes", a.listRoutes)
	auth.POST("/routes", a.saveRoute)
	auth.PUT("/routes/:id", a.saveRoute)
	auth.DELETE("/routes/:id", a.deleteRoute)

	auth.GET("/messages", a.listMessages)
	auth.GET("/messages/:id", a.getMessage)
	auth.POST("/messages/:id/retry", a.retryMessage)
	auth.DELETE("/messages/:id", a.deleteMessage)

	auth.GET("/dashboard", a.dashboard)

	auth.GET("/settings", a.getSettings)
	auth.PUT("/settings", a.saveSettings)
	auth.PUT("/settings/password", a.changePassword)
}

// ---------- 认证 ----------

func (a *API) initialized() bool {
	return store.GetSetting(a.db, "password_hash") != ""
}

func (a *API) meta(c *gin.Context) {
	c.JSON(200, gin.H{
		"initialized": a.initialized(),
		"authed":      a.authed(c),
	})
}

func (a *API) authed(c *gin.Context) bool {
	tok, err := c.Cookie("mg_session")
	if err != nil || tok == "" {
		return false
	}
	v, ok := a.sessions.Load(tok)
	if !ok {
		return false
	}
	return time.Now().Before(v.(time.Time))
}

func (a *API) requireAuth(c *gin.Context) {
	if !a.authed(c) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录或会话已过期"})
		return
	}
	c.Next()
}

type credReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *API) init(c *gin.Context) {
	if a.initialized() {
		c.JSON(400, gin.H{"error": "已初始化,请直接登录"})
		return
	}
	var req credReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || len(req.Password) < 6 {
		c.JSON(400, gin.H{"error": "用户名不能为空,密码至少 6 位"})
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	_ = store.SetSetting(a.db, "account", req.Username)
	_ = store.SetSetting(a.db, "password_hash", string(hash))
	a.startSession(c)
	c.JSON(200, gin.H{"ok": true})
}

func (a *API) login(c *gin.Context) {
	var req credReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "参数错误"})
		return
	}
	account := store.GetSetting(a.db, "account")
	hash := store.GetSetting(a.db, "password_hash")
	if account == "" || hash == "" {
		c.JSON(400, gin.H{"error": "尚未初始化"})
		return
	}
	if subtleEqual(req.Username, account) && bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) == nil {
		a.startSession(c)
		c.JSON(200, gin.H{"ok": true})
		return
	}
	time.Sleep(300 * time.Millisecond) // 轻量防爆破
	c.JSON(401, gin.H{"error": "用户名或密码错误"})
}

func subtleEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func (a *API) startSession(c *gin.Context) {
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	tok := hex.EncodeToString(buf)
	a.sessions.Store(tok, time.Now().Add(24*time.Hour))
	c.SetCookie("mg_session", tok, 86400, "/", "", false, true)
}

func (a *API) logout(c *gin.Context) {
	if tok, err := c.Cookie("mg_session"); err == nil {
		a.sessions.Delete(tok)
	}
	c.JSON(200, gin.H{"ok": true})
}

func (a *API) me(c *gin.Context) {
	c.JSON(200, gin.H{"username": store.GetSetting(a.db, "account")})
}

// ---------- 入站通道 ----------

func (a *API) listSources(c *gin.Context) {
	var list []store.Source
	a.db.Order("id").Find(&list)
	c.JSON(200, list)
}

func (a *API) saveSource(c *gin.Context) {
	var s store.Source
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(400, gin.H{"error": "参数错误"})
		return
	}
	s.Code = strings.ToLower(strings.TrimSpace(s.Code))
	if !inbound.ValidCode(s.Code) {
		c.JSON(400, gin.H{"error": "通道标识只能是 1-32 位小写字母/数字/连字符"})
		return
	}
	var cnt int64
	a.db.Model(&store.Source{}).Where("code = ? AND id <> ?", s.Code, s.ID).Count(&cnt)
	if cnt > 0 {
		c.JSON(400, gin.H{"error": "通道标识已存在: " + s.Code})
		return
	}
	if !sourceTypes[s.Type] {
		c.JSON(400, gin.H{"error": "不支持的入站类型"})
		return
	}
	if err := a.db.Omit("created_at").Save(&s).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	a.db.First(&s, s.ID)
	c.JSON(200, s)
}

var sourceTypes = map[string]bool{"gotify": true, "ntfy": true, "bark": true, "webhook": true, "telegram": true, "wecom": true, "dingtalk": true}

func (a *API) deleteSource(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	a.db.Delete(&store.Source{}, id)
	c.JSON(200, gin.H{"ok": true})
}

// ---------- 出站通道 ----------

func (a *API) listTargets(c *gin.Context) {
	var list []store.Target
	a.db.Order("id").Find(&list)
	c.JSON(200, list)
}

func (a *API) saveTarget(c *gin.Context) {
	var t store.Target
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(400, gin.H{"error": "参数错误"})
		return
	}
	if !adapter.HasSender(t.Type) {
		c.JSON(400, gin.H{"error": "不支持的出站类型: " + t.Type})
		return
	}
	if strings.TrimSpace(t.Name) == "" {
		c.JSON(400, gin.H{"error": "名称不能为空"})
		return
	}
	if err := a.db.Omit("created_at").Save(&t).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	a.db.First(&t, t.ID)
	c.JSON(200, t)
}

func (a *API) deleteTarget(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	a.db.Transaction(func(tx *gorm.DB) error {
		tx.Where("target_id = ?", id).Delete(&store.RouteTarget{})
		tx.Delete(&store.Target{}, id)
		return nil
	})
	c.JSON(200, gin.H{"ok": true})
}

func (a *API) testTarget(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var t store.Target
	if err := a.db.First(&t, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "通道不存在"})
		return
	}
	now := time.Now()
	msg := &adapter.Message{
		Title:    "massage-gate 测试消息",
		Body:     fmt.Sprintf("出站通道「%s」测试,发送于 %s", t.Name, now.Format("15:04:05")),
		Priority: 5,
		Time:     now,
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	if err := adapter.Send(ctx, t.Type, adapter.ParseConfig(t.Config), msg); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// ---------- 路由 ----------

type routePayload struct {
	store.Route
	Targets []store.RouteTarget `json:"targets"`
}

func (a *API) listRoutes(c *gin.Context) {
	var routes []store.Route
	a.db.Order("id").Find(&routes)
	var rts []store.RouteTarget
	a.db.Find(&rts)
	out := make([]routePayload, 0, len(routes))
	for _, rt := range routes {
		p := routePayload{Route: rt, Targets: []store.RouteTarget{}}
		for _, t := range rts {
			if t.RouteID == rt.ID {
				p.Targets = append(p.Targets, t)
			}
		}
		sort.Slice(p.Targets, func(i, j int) bool {
			if p.Targets[i].Role != p.Targets[j].Role {
				return p.Targets[i].Role == "primary"
			}
			return p.Targets[i].Sort < p.Targets[j].Sort
		})
		out = append(out, p)
	}
	c.JSON(200, out)
}

func (a *API) saveRoute(c *gin.Context) {
	var p routePayload
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(400, gin.H{"error": "参数错误"})
		return
	}
	if p.SourceID == 0 {
		c.JSON(400, gin.H{"error": "必须选择入站通道"})
		return
	}
	var sc int64
	a.db.Model(&store.Source{}).Where("id = ?", p.SourceID).Count(&sc)
	if sc == 0 {
		c.JSON(400, gin.H{"error": "入站通道不存在"})
		return
	}
	if p.PriorityMode == "map" {
		if p.PriorityMap != "" {
			m := map[string]int{}
			if err := json.Unmarshal([]byte(p.PriorityMap), &m); err != nil {
				c.JSON(400, gin.H{"error": "优先级映射必须是 JSON 对象,如 {\"5\":3}"})
				return
			}
		}
	} else if p.PriorityMode == "" {
		p.PriorityMode = "passthrough"
	}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		q := tx
		if p.ID != 0 {
			q = tx.Omit("created_at")
		}
		if err := q.Save(&p.Route).Error; err != nil {
			return err
		}
		if err := tx.Where("route_id = ?", p.ID).Delete(&store.RouteTarget{}).Error; err != nil {
			return err
		}
		seen := map[uint]bool{}
		for i, t := range p.Targets {
			if t.TargetID == 0 || seen[t.TargetID] {
				continue
			}
			seen[t.TargetID] = true
			nt := store.RouteTarget{RouteID: p.ID, TargetID: t.TargetID, Role: t.Role, Sort: t.Sort}
			if nt.Role != "backup" {
				nt.Role = "primary"
			}
			if nt.Sort == 0 {
				nt.Sort = i
			}
			if err := tx.Create(&nt).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, p.Route)
}

func (a *API) deleteRoute(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	a.db.Transaction(func(tx *gorm.DB) error {
		tx.Where("route_id = ?", id).Delete(&store.RouteTarget{})
		tx.Delete(&store.Route{}, id)
		return nil
	})
	c.JSON(200, gin.H{"ok": true})
}

// ---------- 消息中心 ----------

func (a *API) listMessages(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	status := c.Query("status")
	sourceID, _ := strconv.Atoi(c.Query("source_id"))
	q := strings.TrimSpace(c.Query("q"))

	qry := a.db.Model(&store.Message{})
	if sourceID > 0 {
		qry = qry.Where("source_id = ?", sourceID)
	}
	if q != "" {
		qry = qry.Where("title LIKE ? OR body LIKE ?", "%"+q+"%", "%"+q+"%")
	}
	if status == "unrouted" {
		qry = qry.Where("id NOT IN (SELECT message_id FROM deliveries)")
	} else if status != "" && status != "all" {
		qry = qry.Where("id IN (SELECT message_id FROM deliveries WHERE status = ?)", status)
	}
	var total int64
	qry.Count(&total)
	var list []store.Message
	qry.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list)

	// 聚合每条消息的投递概要
	type summary struct {
		MessageID uint
		Total     int64
		Success   int64
		Queued    int64
		Pending   int64
		Dead      int64
	}
	sums := map[uint]*summary{}
	if len(list) > 0 {
		ids := make([]uint, len(list))
		for i, m := range list {
			ids[i] = m.ID
		}
		var rows []summary
		a.db.Raw(`SELECT message_id,
			COUNT(*) AS total,
			SUM(CASE WHEN status='success' THEN 1 ELSE 0 END) AS success,
			SUM(CASE WHEN status='queued' THEN 1 ELSE 0 END) AS queued,
			SUM(CASE WHEN status='pending' OR status='sending' THEN 1 ELSE 0 END) AS pending,
			SUM(CASE WHEN status='dead' THEN 1 ELSE 0 END) AS dead
			FROM deliveries WHERE message_id IN ? GROUP BY message_id`, ids).Scan(&rows)
		for i := range rows {
			sums[rows[i].MessageID] = &rows[i]
		}
	}
	out := make([]gin.H, 0, len(list))
	for _, m := range list {
		item := gin.H{
			"id": m.ID, "title": m.Title, "body": m.Body, "priority": m.Priority,
			"tags": m.Tags, "received_at": m.ReceivedAt, "source_id": m.SourceID,
		}
		if s, ok := sums[m.ID]; ok {
			st := "pending"
			switch {
			case s.Success > 0:
				st = "success"
			case s.Queued > 0:
				st = "queued"
			case s.Pending > 0:
				st = "pending"
			case s.Dead > 0:
				st = "dead"
			}
			item["status"] = st
			item["delivery_total"] = s.Total
		} else {
			item["status"] = "unrouted"
			item["delivery_total"] = 0
		}
		out = append(out, item)
	}
	c.JSON(200, gin.H{"list": out, "total": total, "page": page, "size": size})
}

func (a *API) getMessage(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var m store.Message
	if err := a.db.First(&m, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "消息不存在"})
		return
	}
	var dels []store.Delivery
	a.db.Where("message_id = ?", m.ID).Order("id").Find(&dels)
	routeNames := map[uint]string{}
	if len(dels) > 0 {
		ids := make([]uint, 0, len(dels))
		for _, d := range dels {
			ids = append(ids, d.RouteID)
		}
		var routes []store.Route
		a.db.Where("id IN ?", ids).Find(&routes)
		for _, r := range routes {
			routeNames[r.ID] = r.Name
		}
	}
	deliveryOut := make([]gin.H, 0, len(dels))
	for _, d := range dels {
		deliveryOut = append(deliveryOut, gin.H{
			"id": d.ID, "route": routeNames[d.RouteID], "status": d.Status,
			"attempts": d.Attempts, "cycles": d.Cycles, "next_at": d.NextAt,
			"last_error": d.LastError, "sent_at": d.SentAt, "created_at": d.CreatedAt,
		})
	}
	c.JSON(200, gin.H{"message": m, "deliveries": deliveryOut})
}

func (a *API) retryMessage(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var cnt int64
	a.db.Model(&store.Delivery{}).Where("message_id = ?", id).Count(&cnt)
	if cnt == 0 {
		c.JSON(400, gin.H{"error": "该消息没有投递任务(未命中路由),无法重试"})
		return
	}
	dispatch.RetryMessage(a.db, uint(id))
	c.JSON(200, gin.H{"ok": true})
}

func (a *API) deleteMessage(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	a.db.Transaction(func(tx *gorm.DB) error {
		tx.Where("message_id = ?", id).Delete(&store.Delivery{})
		tx.Delete(&store.Message{}, id)
		return nil
	})
	c.JSON(200, gin.H{"ok": true})
}

// ---------- 仪表盘 ----------

func (a *API) dashboard(c *gin.Context) {
	var msgTotal, msg24h int64
	a.db.Model(&store.Message{}).Count(&msgTotal)
	a.db.Model(&store.Message{}).Where("received_at > ?", time.Now().Add(-24*time.Hour)).Count(&msg24h)

	type cntRow struct {
		Status string
		N      int64
	}
	var rows []cntRow
	a.db.Model(&store.Delivery{}).Select("status, COUNT(*) AS n").Group("status").Scan(&rows)
	delCounts := map[string]int64{}
	for _, r := range rows {
		delCounts[r.Status] = r.N
	}
	var recent []store.Message
	a.db.Order("id DESC").Limit(10).Find(&recent)

	var srcOn, tgtOn int64
	a.db.Model(&store.Source{}).Where("enabled = ?", true).Count(&srcOn)
	a.db.Model(&store.Target{}).Where("enabled = ?", true).Count(&tgtOn)

	// 最近投递失败原因(去重前 5 条)
	var failMsgs []string
	a.db.Model(&store.Delivery{}).Where("last_error <> ''").Order("id DESC").Limit(5).
		Pluck("last_error", &failMsgs)

	c.JSON(200, gin.H{
		"messages_total":  msgTotal,
		"messages_24h":    msg24h,
		"del_pending":     delCounts[store.DeliveryPending] + delCounts[store.DeliverySending],
		"del_success":     delCounts[store.DeliverySuccess],
		"del_queued":      delCounts[store.DeliveryQueued],
		"del_dead":        delCounts[store.DeliveryDead],
		"sources_enabled": srcOn,
		"targets_enabled": tgtOn,
		"recent":          recent,
		"recent_errors":   failMsgs,
	})
}

// ---------- 设置 ----------

var settingKeys = []string{"keep_days", "timeout_default_ms", "backoff_base_sec", "backoff_max_sec"}

func (a *API) getSettings(c *gin.Context) {
	out := gin.H{"username": store.GetSetting(a.db, "account")}
	for _, k := range settingKeys {
		out[k] = store.GetSetting(a.db, k)
	}
	c.JSON(200, out)
}

func (a *API) saveSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "参数错误"})
		return
	}
	for _, k := range settingKeys {
		if v, ok := req[k]; ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err != nil || n < 0 {
				c.JSON(400, gin.H{"error": k + " 必须是非负整数"})
				return
			}
			_ = store.SetSetting(a.db, k, strings.TrimSpace(v))
		}
	}
	c.JSON(200, gin.H{"ok": true})
}

func (a *API) changePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "参数错误"})
		return
	}
	hash := store.GetSetting(a.db, "password_hash")
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.OldPassword)) != nil {
		c.JSON(400, gin.H{"error": "原密码错误"})
		return
	}
	if len(req.NewPassword) < 6 {
		c.JSON(400, gin.H{"error": "新密码至少 6 位"})
		return
	}
	nh, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	_ = store.SetSetting(a.db, "password_hash", string(nh))
	c.JSON(200, gin.H{"ok": true})
}
