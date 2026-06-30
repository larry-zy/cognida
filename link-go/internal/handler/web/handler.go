// Package web 提供静态文件服务
package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// Handler 静态文件处理器
type Handler struct {
	staticDir string
}

// NewHandler 创建静态文件处理器
func NewHandler(staticDir string) *Handler {
	return &Handler{
		staticDir: staticDir,
	}
}

// ServeFile 处理静态文件请求
func (h *Handler) ServeFile(c *gin.Context) {
	// 获取请求路径
	requestPath := c.Param("filepath")

	// 如果路径为空或根路径，返回 index.html
	if requestPath == "" || requestPath == "/" {
		requestPath = "/index.html"
	}

	// 移除前导斜杠
	requestPath = strings.TrimPrefix(requestPath, "/")

	// 构建完整文件路径
	filePath := filepath.Join(h.staticDir, requestPath)

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// 如果文件不存在，尝试添加 .html 后缀（SPA 路由支持）
		if !strings.Contains(filepath.Base(requestPath), ".") {
			htmlPath := filepath.Join(h.staticDir, requestPath+".html")
			if _, err := os.Stat(htmlPath); err == nil {
				c.File(htmlPath)
				return
			}
		}
		// 文件不存在，返回 404
		c.Status(http.StatusNotFound)
		return
	}

	// 返回文件
	c.File(filePath)
}

// Index 首页
func (h *Handler) Index(c *gin.Context) {
	c.File(filepath.Join(h.staticDir, "index.html"))
}
