//go:build dev

package web

import (
	"html/template"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// IsEmbedded 返回是否使用嵌入资源
func IsEmbedded() bool {
	return false
}

// getBaseDir 获取基础目录 (可执行文件所在目录)
func getBaseDir() string {
	// 优先使用环境变量
	if dir := os.Getenv("EZPAY_BASE_DIR"); dir != "" {
		return dir
	}

	// 获取可执行文件所在目录
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// LoadTemplates 加载模板 (开发模式 - 从文件系统)
func LoadTemplates(r *gin.Engine, funcMap template.FuncMap) error {
	baseDir := getBaseDir()
	templatePath := filepath.Join(baseDir, "web", "templates", "*")

	r.SetFuncMap(funcMap)
	r.LoadHTMLGlob(templatePath)
	return nil
}

// SetupStatic 设置静态文件服务 (开发模式 - 从文件系统)
func SetupStatic(r *gin.Engine, dataDir string) error {
	baseDir := getBaseDir()

	// 静态资源目录
	staticPath := filepath.Join(baseDir, "web", "static")
	r.Static("/static/css", filepath.Join(staticPath, "css"))
	r.Static("/static/js", filepath.Join(staticPath, "js"))
	r.Static("/static/fonts", filepath.Join(staticPath, "fonts"))
	r.Static("/static/webfonts", filepath.Join(staticPath, "webfonts"))
	r.Static("/static/locales", filepath.Join(staticPath, "locales"))
	r.Static("/static/img", filepath.Join(staticPath, "img"))

	// 上传目录 (使用配置的数据目录)
	os.MkdirAll(dataDir+"/qrcode", 0755)
	os.MkdirAll(dataDir+"/apk", 0755)

	r.Static("/static/qrcode", dataDir+"/qrcode")
	r.Static("/static/uploads", dataDir)

	return nil
}

// GetUploadDir 获取上传目录路径
func GetUploadDir() string {
	baseDir := getBaseDir()
	return filepath.Join(baseDir, "uploads")
}
