// Package dispatch 投递调度器:
//   - 轮询到期的投递任务,逐目标发送(带每通道超时)
//   - 同一目标失败至 MaxRetry 次 → 主备切换
//   - 主备链全部失败 → 状态转为"暂存",按指数退避持续重试,保证消息不丢
package dispatch

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"massagegate/internal/adapter"
	"massagegate/internal/store"
)

type Dispatcher struct {
	DB *gorm.DB
}

func Run(db *gorm.DB) {
	d := &Dispatcher{DB: db}
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	lastCleanup := time.Now()
	for range t.C {
		d.tick()
		if time.Since(lastCleanup) > time.Hour {
			d.cleanup()
			lastCleanup = time.Now()
		}
	}
}

func (d *Dispatcher) tick() {
	now := time.Now()
	var ids []uint
	d.DB.Model(&store.Delivery{}).
		Where("status IN ? AND next_at <= ?", []string{store.DeliveryPending, store.DeliveryQueued}, now).
		Order("next_at").Limit(16).Pluck("id", &ids)
	if len(ids) == 0 {
		return
	}
	d.DB.Model(&store.Delivery{}).Where("id IN ?", ids).Update("status", store.DeliverySending)
	var dels []store.Delivery
	d.DB.Where("id IN ?", ids).Find(&dels)
	// 并发上限 4,WaitGroup 等待全部完成
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i := range dels {
		wg.Add(1)
		sem <- struct{}{}
		go func(dl store.Delivery) {
			defer wg.Done()
			defer func() { <-sem }()
			d.process(dl)
		}(dels[i])
	}
	wg.Wait()
}

// Chain 取路由的目标链:主目标(按 Sort)在前,备目标在后
func (d *Dispatcher) Chain(routeID uint) []*store.Target {
	var rts []store.RouteTarget
	d.DB.Where("route_id = ?", routeID).Find(&rts)
	var primaries, backups []store.RouteTarget
	for _, rt := range rts {
		if rt.Role == "backup" {
			backups = append(backups, rt)
		} else {
			primaries = append(primaries, rt)
		}
	}
	sortTargets(primaries)
	sortTargets(backups)
	ids := make([]uint, 0, len(primaries)+len(backups))
	for _, rt := range primaries {
		ids = append(ids, rt.TargetID)
	}
	for _, rt := range backups {
		ids = append(ids, rt.TargetID)
	}
	if len(ids) == 0 {
		return nil
	}
	var targets []store.Target
	d.DB.Where("id IN ?", ids).Find(&targets)
	byID := map[uint]*store.Target{}
	for i := range targets {
		byID[targets[i].ID] = &targets[i]
	}
	chain := make([]*store.Target, 0, len(ids))
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			chain = append(chain, t)
		}
	}
	return chain
}

func sortTargets(list []store.RouteTarget) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j].Sort < list[j-1].Sort; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

// process 对一个投递任务执行一轮尝试(含目标内重试与主备切换)
func (d *Dispatcher) process(dl store.Delivery) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[dispatch] panic delivery=%d: %v", dl.ID, r)
			// 复位为待投递,避免永久卡在 sending
			d.DB.Model(&store.Delivery{}).Where("id = ?", dl.ID).
				Updates(map[string]any{"status": store.DeliveryPending, "next_at": time.Now().Add(time.Minute)})
		}
	}()

	var msg store.Message
	if err := d.DB.First(&msg, dl.MessageID).Error; err != nil {
		d.DB.Model(&dl).Updates(map[string]any{"status": store.DeliveryDead, "last_error": "消息不存在(可能已清理)"})
		return
	}
	var route store.Route
	if err := d.DB.First(&route, dl.RouteID).Error; err != nil {
		d.DB.Model(&dl).Updates(map[string]any{"status": store.DeliveryDead, "last_error": "路由不存在(可能已删除)"})
		return
	}
	chain := d.Chain(route.ID)
	if len(chain) == 0 {
		d.DB.Model(&dl).Updates(map[string]any{"status": store.DeliveryDead, "last_error": "路由未配置可用目标"})
		return
	}

	out := transform(route, msg, d)
	idx := dl.TargetIdx
	if idx < 0 || idx >= len(chain) {
		idx = 0
	}
	now := time.Now()
	for i := idx; i < len(chain); i++ {
		t := chain[i]
		if !t.Enabled {
			continue // 跳过已停用目标,视为切换到下一个
		}
		tries := t.MaxRetry
		if tries < 1 {
			tries = 1
		}
		var lastErr error
		for a := 0; a < tries; a++ {
			err := d.sendOne(t, out)
			dl.Attempts++
			if err == nil {
				dl.Status = store.DeliverySuccess
				dl.LastError = ""
				dl.NextAt = now
				sentAt := now
				dl.SentAt = &sentAt
				d.DB.Save(&dl)
				return
			}
			lastErr = err
			log.Printf("[dispatch] 投递失败 delivery=%d target=%s: %v", dl.ID, t.Name, err)
		}
		dl.LastError = lastErr.Error()
		// 此目标重试耗尽 -> 主备切换,继续尝试链上下一目标
	}
	// 主备全部失败 -> 暂存,指数退避持续重试(消息不丢)
	dl.Status = store.DeliveryQueued
	dl.TargetIdx = 0
	dl.NextAt = now.Add(d.backoff(dl.Cycles))
	dl.Cycles++
	d.DB.Save(&dl)
	log.Printf("[dispatch] 主备全部失败,暂存重试 delivery=%d cycles=%d next=%s",
		dl.ID, dl.Cycles, dl.NextAt.Format("15:04:05"))
}

func (d *Dispatcher) sendOne(t *store.Target, out *adapter.Message) error {
	timeoutMs := t.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 10000
		if s := store.GetSetting(d.DB, "timeout_default_ms"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				timeoutMs = n
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()
	return adapter.Send(ctx, t.Type, adapter.ParseConfig(t.Config), out)
}

// backoff 指数退避:base * 2^cycles,上限 max
func (d *Dispatcher) backoff(cycles int) time.Duration {
	base, maxSec := 60, 3600
	if s := store.GetSetting(d.DB, "backoff_base_sec"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			base = n
		}
	}
	if s := store.GetSetting(d.DB, "backoff_max_sec"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxSec = n
		}
	}
	sec := base << min(cycles, 20)
	if sec > maxSec {
		sec = maxSec
	}
	return time.Duration(sec) * time.Second
}

// cleanup 按保留天数清理历史消息与投递记录
func (d *Dispatcher) cleanup() {
	keepDays := 30
	if s := store.GetSetting(d.DB, "keep_days"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			keepDays = n
		}
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays)
	d.DB.Where("received_at < ?", cutoff).Delete(&store.Message{})
	d.DB.Exec(`DELETE FROM deliveries WHERE message_id NOT IN (SELECT id FROM messages)`)
}

// transform 应用路由的转换:标题/正文模板、优先级映射
func transform(route store.Route, msg store.Message, d *Dispatcher) *adapter.Message {
	out := &adapter.Message{
		Title:    msg.Title,
		Body:     msg.Body,
		Priority: msg.Priority,
		Time:     msg.ReceivedAt,
	}
	if msg.Tags != "" {
		out.Tags = strings.Split(msg.Tags, ",")
	}
	if msg.ClickURL != "" {
		out.ClickURL = msg.ClickURL
	}
	var srcName string
	var src store.Source
	if err := d.DB.First(&src, msg.SourceID).Error; err == nil {
		srcName = src.Name
	}
	if route.TitleTpl != "" {
		out.Title = renderTpl(route.TitleTpl, out, srcName)
	}
	if route.BodyTpl != "" {
		out.Body = renderTpl(route.BodyTpl, out, srcName)
	}
	switch route.PriorityMode {
	case "fixed":
		out.Priority = route.PriorityValue
	case "map":
		m := map[string]int{}
		if json.Unmarshal([]byte(route.PriorityMap), &m) == nil {
			if v, ok := m[strconv.Itoa(msg.Priority)]; ok {
				out.Priority = v
			}
		}
	}
	return out
}

func renderTpl(tpl string, msg *adapter.Message, srcName string) string {
	return strings.NewReplacer(
		"{{title}}", msg.Title,
		"{{body}}", msg.Body,
		"{{priority}}", strconv.Itoa(msg.Priority),
		"{{time}}", msg.Time.Local().Format("2006-01-02 15:04:05"),
		"{{source}}", srcName,
	).Replace(tpl)
}

// RetryMessage 手动重试:重置该消息全部投递任务
func RetryMessage(db *gorm.DB, messageID uint) {
	now := time.Now()
	db.Model(&store.Delivery{}).Where("message_id = ?", messageID).Updates(map[string]any{
		"status":     store.DeliveryPending,
		"target_idx": 0,
		"cycles":     0,
		"next_at":    now,
		"last_error": "",
		"sent_at":    nil,
	})
}

// ResetSending 服务启动时把上次进程中断遗留的 sending 任务复位
func ResetSending(db *gorm.DB) {
	db.Model(&store.Delivery{}).Where("status = ?", store.DeliverySending).
		Updates(map[string]any{"status": store.DeliveryPending, "next_at": time.Now()})
}
