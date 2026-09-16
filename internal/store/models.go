package store

import "time"

// Setting 键值配置(账号、全局参数等)
type Setting struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `json:"value"`
}

// Source 入站通道(消息来源,一个 code 对外暴露 /i/{code})
type Source struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128" json:"name"`
	Code      string    `gorm:"uniqueIndex;size:64" json:"code"`
	Type      string    `gorm:"size:32" json:"type"` // gotify|ntfy|bark|webhook|telegram|wecom|dingtalk
	Config    string    `gorm:"type:text" json:"config"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// Target 出站通道(推送目的地)
type Target struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128" json:"name"`
	Type      string    `gorm:"size:32" json:"type"` // gotify|ntfy|bark|telegram|webhook|custom
	Config    string    `gorm:"type:text" json:"config"`
	TimeoutMs int       `json:"timeout_ms"` // 单次请求超时,0=用全局默认
	MaxRetry  int       `json:"max_retry"`  // 单轮投递中同一目标最多尝试次数,<1 视为 1
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// Route 路由规则:入站通道 -> 出站目标(主/备)
type Route struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Name          string    `gorm:"size:128" json:"name"`
	SourceID      uint      `gorm:"index" json:"source_id"`
	Enabled       bool      `json:"enabled"`
	FilterKeyword string    `gorm:"size:255" json:"filter_keyword"` // 非空时,标题/正文须包含该关键字
	PriorityMode  string    `gorm:"size:16" json:"priority_mode"`   // passthrough|fixed|map
	PriorityValue int       `json:"priority_value"`
	PriorityMap   string    `gorm:"type:text" json:"priority_map"` // JSON: {"5":3}
	TitleTpl      string    `gorm:"type:text" json:"title_tpl"`
	BodyTpl       string    `gorm:"type:text" json:"body_tpl"`
	CreatedAt     time.Time `json:"created_at"`
}

// RouteTarget 路由目标,Role: primary|backup,Sort 升序
type RouteTarget struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	RouteID  uint   `gorm:"index" json:"route_id"`
	TargetID uint   `json:"target_id"`
	Role     string `gorm:"size:16" json:"role"`
	Sort     int    `json:"sort"`
}

// Message 收到的标准化消息(先落库,保证不丢)
type Message struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SourceID   uint      `gorm:"index" json:"source_id"`
	Title      string    `json:"title"`
	Body       string    `gorm:"type:text" json:"body"`
	Priority   int       `json:"priority"`
	Tags       string    `gorm:"size:255" json:"tags"`
	ClickURL   string    `gorm:"size:512" json:"click_url"`
	Raw        string    `gorm:"type:text" json:"raw"`
	ReceivedAt time.Time `gorm:"index" json:"received_at"`
}

// Delivery 投递任务(每条消息 x 每条命中路由一条),主备切换与暂存重试都在其上推进
type Delivery struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	MessageID uint       `gorm:"index" json:"message_id"`
	RouteID   uint       `gorm:"index" json:"route_id"`
	TargetIdx int        `json:"target_idx"` // 当前在目标链(主->备)中的位置
	Cycles    int        `json:"cycles"`     // 整链失败进入暂存的轮数,用于退避
	Attempts  int        `json:"attempts"`   // 累计发送次数
	Status    string     `gorm:"index;size:16" json:"status"`
	NextAt    time.Time  `gorm:"index" json:"next_at"`
	LastError string     `gorm:"type:text" json:"last_error"`
	CreatedAt time.Time  `json:"created_at"`
	SentAt    *time.Time `json:"sent_at"`
}

// 投递状态
const (
	DeliveryPending = "pending" // 待投递
	DeliverySending = "sending" // 投递中
	DeliverySuccess = "success" // 已送达
	DeliveryQueued  = "queued"  // 暂存(主备全失败,退避重试中)
	DeliveryDead    = "dead"    // 无法投递(如路由无可用目标)
)
