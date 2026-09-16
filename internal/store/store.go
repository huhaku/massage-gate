package store

import (
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open 打开(或创建)SQLite 数据库并迁移表结构
func Open(path string) (*gorm.DB, error) {
	dsn := "file:" + filepath.ToSlash(path) +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// 单连接串行化访问,规避 SQLite 写锁竞争
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&Setting{}, &Source{}, &Target{}, &Route{}, &RouteTarget{}, &Message{}, &Delivery{}); err != nil {
		return nil, err
	}
	return db, nil
}

func GetSetting(db *gorm.DB, key string) string {
	var s Setting
	if err := db.Where("key = ?", key).First(&s).Error; err != nil {
		return ""
	}
	return s.Value
}

func SetSetting(db *gorm.DB, key, value string) error {
	return db.Save(&Setting{Key: key, Value: value}).Error
}

func SetSettingDefault(db *gorm.DB, key, value string) {
	if GetSetting(db, key) == "" {
		_ = SetSetting(db, key, value)
	}
}
