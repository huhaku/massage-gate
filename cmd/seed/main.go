// cmd/seed 向数据库注入示例数据(幂等:已存在的通道/路由会跳过)。
// 用法: go run ./cmd/seed -data ./data
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"massagegate/internal/store"
)

func main() {
	dataDir := flag.String("data", "./data", "数据目录")
	clean := flag.Bool("clean", false, "仅清空消息与投递记录(不影响通道/路由配置)")
	flag.Parse()

	db, err := store.Open(filepath.Join(*dataDir, "massage-gate.db"))
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}

	if *clean {
		r1 := db.Where("1 = 1").Delete(&store.Delivery{}).RowsAffected
		r2 := db.Where("1 = 1").Delete(&store.Message{}).RowsAffected
		fmt.Printf("已清空投递记录 %d 条、消息 %d 条。\n", r1, r2)
		return
	}

	cfgJSON := func(m map[string]any) string {
		b, _ := json.Marshal(m)
		return string(b)
	}

// ---- 入站通道 ----
		sources := []store.Source{
			{Name: "Gotify 演示源", Code: "gotify-demo", Type: "gotify", Enabled: true,
				Config: cfgJSON(map[string]any{"token": "demo-token"})},
			{Name: "ntfy 演示源", Code: "ntfy-demo", Type: "ntfy", Enabled: true, Config: "{}"},
			{Name: "Webhook 演示源", Code: "webhook-demo", Type: "webhook", Enabled: true, Config: "{}"},
			{Name: "Telegram Bot 演示源", Code: "tg-demo", Type: "telegram", Enabled: true,
				Config: cfgJSON(map[string]any{"bot_token": "123456:ABC-DEMO"})},
			{Name: "企业微信演示源", Code: "wecom-demo", Type: "wecom", Enabled: true,
				Config: cfgJSON(map[string]any{"token": "wecom-token", "encoding_aes_key": "demo-aes-key-12345678901234567890"})},
			{Name: "钉钉机器人演示源", Code: "dingtalk-demo", Type: "dingtalk", Enabled: true,
				Config: cfgJSON(map[string]any{"token": "dingtalk-token", "secret": "demo-secret"})},
		}
	srcID := map[string]uint{}
	for _, s := range sources {
		var cnt int64
		db.Model(&store.Source{}).Where("code = ?", s.Code).Count(&cnt)
		if cnt > 0 {
			db.Where("code = ?", s.Code).First(&s)
			srcID[s.Code] = s.ID
			continue
		}
		if err := db.Create(&s).Error; err != nil {
			log.Fatalf("创建入站通道失败: %v", err)
		}
		srcID[s.Code] = s.ID
		fmt.Printf("入站通道 + %s (code=%s)\n", s.Name, s.Code)
	}

	// ---- 出站通道 ----
	targets := []store.Target{
		{Name: "本地接收端(可用)", Type: "webhook", Enabled: true, TimeoutMs: 3000, MaxRetry: 2,
			Config: cfgJSON(map[string]any{"url": "http://127.0.0.1:9991/hook"})},
		{Name: "故障演示目标(不可达)", Type: "webhook", Enabled: true, TimeoutMs: 2000, MaxRetry: 2,
			Config: cfgJSON(map[string]any{"url": "http://127.0.0.1:9992/hook"})},
		{Name: "故障演示目标B(不可达)", Type: "webhook", Enabled: true, TimeoutMs: 2000, MaxRetry: 2,
			Config: cfgJSON(map[string]any{"url": "http://127.0.0.1:9993/hook"})},
		{Name: "自定义模板目标(企业微信格式)", Type: "custom", Enabled: true, TimeoutMs: 3000, MaxRetry: 2,
			Config: cfgJSON(map[string]any{
				"url":           "http://127.0.0.1:9991/wecom",
				"method":        "POST",
				"headers":       `{"X-Demo":"massage-gate"}`,
				"body_template": `{"msgtype":"text","text":{"content":"{{title}} | {{body}}"}}`,
			})},
		{Name: "企业微信演示机器人", Type: "wecom", Enabled: true, TimeoutMs: 3000, MaxRetry: 2,
			Config: cfgJSON(map[string]any{"url": "http://127.0.0.1:9991/wecom", "msg_type": "text"})},
		{Name: "飞书演示机器人", Type: "feishu", Enabled: true, TimeoutMs: 3000, MaxRetry: 2,
			Config: cfgJSON(map[string]any{"url": "http://127.0.0.1:9991/feishu", "secret": "demo-secret"})},
		{Name: "钉钉演示机器人", Type: "dingtalk", Enabled: true, TimeoutMs: 3000, MaxRetry: 2,
			Config: cfgJSON(map[string]any{"url": "http://127.0.0.1:9991/dingtalk", "secret": "demo-secret"})},
	}
	tgtID := map[string]uint{}
	for _, t := range targets {
		var cnt int64
		db.Model(&store.Target{}).Where("name = ?", t.Name).Count(&cnt)
		if cnt > 0 {
			db.Where("name = ?", t.Name).First(&t)
			tgtID[t.Name] = t.ID
			continue
		}
		if err := db.Create(&t).Error; err != nil {
			log.Fatalf("创建出站通道失败: %v", err)
		}
		tgtID[t.Name] = t.ID
		fmt.Printf("出站通道 + %s (%s)\n", t.Name, t.Type)
	}

	// ---- 路由(主备链) ----
	type rt struct{ target, role string }
	routes := []struct {
		name, source, filter, titleTpl string
		chain                          []rt
	}{
		{name: "主备切换演示", source: "gotify-demo", chain: []rt{
			{"故障演示目标(不可达)", "primary"}, {"本地接收端(可用)", "backup"}}},
		{name: "告警关键字转发", source: "ntfy-demo", filter: "告警", chain: []rt{
			{"本地接收端(可用)", "primary"}}},
		{name: "模板转换演示", source: "webhook-demo", titleTpl: "【{{source}}】{{title}}", chain: []rt{
			{"自定义模板目标(企业微信格式)", "primary"}}},
		{name: "全失败暂存演示", source: "webhook-demo", chain: []rt{
			{"故障演示目标(不可达)", "primary"}, {"故障演示目标B(不可达)", "backup"}}},
		{name: "企微机器人演示", source: "webhook-demo", chain: []rt{
			{"企业微信演示机器人", "primary"}}},
		{name: "飞书机器人演示", source: "webhook-demo", chain: []rt{
			{"飞书演示机器人", "primary"}}},
{name: "钉钉机器人演示", source: "webhook-demo", chain: []rt{
				{"钉钉演示机器人", "primary"}}},
			{name: "Telegram 入站演示", source: "tg-demo", chain: []rt{
				{"本地接收端(可用)", "primary"}}},
			{name: "企业微信入站演示", source: "wecom-demo", chain: []rt{
				{"本地接收端(可用)", "primary"}}},
			{name: "钉钉入站演示", source: "dingtalk-demo", chain: []rt{
				{"本地接收端(可用)", "primary"}}},
		}
	for _, r := range routes {
		var cnt int64
		db.Model(&store.Route{}).Where("name = ?", r.name).Count(&cnt)
		if cnt > 0 {
			continue
		}
		route := store.Route{
			Name: r.name, SourceID: srcID[r.source], Enabled: true,
			FilterKeyword: r.filter, PriorityMode: "passthrough", TitleTpl: r.titleTpl,
		}
		if err := db.Create(&route).Error; err != nil {
			log.Fatalf("创建路由失败: %v", err)
		}
		for i, c := range r.chain {
			link := store.RouteTarget{RouteID: route.ID, TargetID: tgtID[c.target], Role: c.role, Sort: i}
			if err := db.Create(&link).Error; err != nil {
				log.Fatalf("创建路由目标失败: %v", err)
			}
		}
		fmt.Printf("路由 + %s (%d 个目标)\n", r.name, len(r.chain))
	}

	fmt.Println("示例数据注入完成。")
}
