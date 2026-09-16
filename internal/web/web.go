// Package web 通过 go:embed 嵌入前端构建产物,提供 SPA 静态服务
package web

import (
	"embed"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var dist embed.FS

func mimeOf(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json", ".map":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	default:
		return "application/octet-stream"
	}
}

func Register(r *gin.Engine) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return
	}
	index, indexErr := fs.ReadFile(sub, "index.html")
	r.NoRoute(func(c *gin.Context) {
		p := strings.TrimPrefix(c.Request.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		data, err := fs.ReadFile(sub, p)
		if err != nil {
			// API 与入站路径保持 404;其余路径回退到 SPA 入口
			if strings.HasPrefix(c.Request.URL.Path, "/api/") ||
				strings.HasPrefix(c.Request.URL.Path, "/i/") ||
				indexErr != nil {
				c.JSON(404, gin.H{"error": "not found"})
				return
			}
			c.Data(200, "text/html; charset=utf-8", index)
			return
		}
		c.Data(200, mimeOf(p), data)
	})
}
