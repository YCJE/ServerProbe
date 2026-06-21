package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:web
var webFS embed.FS

// GetEmbeddedFS 返回内嵌的前端文件系统
func GetEmbeddedFS() (fs.FS, error) {
	return fs.Sub(webFS, "web")
}

// StaticFileHandler 静态文件处理器（兼容 gin.HandlerFunc）
func StaticFileHandler() gin.HandlerFunc {
	subFS, err := GetEmbeddedFS()
	if err != nil {
		panic("内嵌前端文件系统初始化失败: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(subFS))

	return func(c *gin.Context) {
		// 检查文件是否存在
		path := strings.TrimPrefix(c.Request.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// 如果文件不存在，返回 index.html（SPA 路由）
		if _, err := fs.Stat(subFS, path); err != nil {
			c.Request.URL.Path = "/"
		}

		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}
