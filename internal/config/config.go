package config

import (
	"flag"
	"os"
	"path/filepath"
)

type Config struct {
	Listen  string // 监听地址
	DataDir string // 数据目录(存 SQLite)
	DBPath  string // SQLite 文件完整路径
}

func Load() *Config {
	c := &Config{}
	flag.StringVar(&c.Listen, "listen", envOr("MG_LISTEN", ":8080"), "HTTP 监听地址")
	flag.StringVar(&c.DataDir, "data", envOr("MG_DATA", "./data"), "数据目录")
	flag.Parse()

	if err := os.MkdirAll(c.DataDir, 0o755); err != nil {
		panic("无法创建数据目录: " + err.Error())
	}
	c.DBPath = filepath.Join(c.DataDir, "massage-gate.db")
	return c
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
