//go:build !dev

package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

//go:embed templates/*
var templateFS embed.FS

// 只嵌入静态资源目录 (css, js, fonts, webfonts, locales, img)
// 不嵌入 uploads 和 qrcode (运行时生成)
//go:embed static/css static/js static/fonts static/webfonts static/locales static/img
var staticFS embed.FS

// IsEmbedded 返回是否使用嵌入资源
func IsEmbedded() bool {
	return true
}

// LoadTemplates 加载模板 (生产模式 - 从嵌入资源)
func LoadTemplates(r *gin.Engine, funcMap template.FuncMap) error {
	tmpl := template.New("").Funcs(funcMap)

	// 遍历嵌入的模板文件
	err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// 读取模板内容
		content, err := templateFS.ReadFile(path)
		if err != nil {
			return err
		}

		// 解析模板
		_, err = tmpl.New(d.Name()).Parse(string(content))
		return err
	})

	if err != nil {
		return err
	}

	r.SetHTMLTemplate(tmpl)
	return nil
}

// SetupStatic 设置静态文件服务 (生产模式 - 从嵌入资源)
func SetupStatic(r *gin.Engine, dataDir string) error {
	// 获取嵌入的 static 子目录
	staticSubFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		return err
	}

	// 嵌入的静态资源 (css, js, fonts, locales, img)
	r.StaticFS("/static/css", http.FS(mustSub(staticSubFS, "css")))
	r.StaticFS("/static/js", http.FS(mustSub(staticSubFS, "js")))
	r.StaticFS("/static/fonts", http.FS(mustSub(staticSubFS, "fonts")))
	r.StaticFS("/static/webfonts", http.FS(mustSub(staticSubFS, "webfonts")))
	r.StaticFS("/static/locales", http.FS(mustSub(staticSubFS, "locales")))
	r.StaticFS("/static/img", http.FS(mustSub(staticSubFS, "img")))

	// 运行时目录 (从文件系统加载，使用配置的数据目录)
	// 确保目录存在
	os.MkdirAll(dataDir+"/qrcode", 0755)
	os.MkdirAll(dataDir+"/apk", 0755)

	r.Static("/static/qrcode", dataDir+"/qrcode")
	r.Static("/static/uploads", dataDir)

	return nil
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

// GetUploadDir 获取上传目录路径
func GetUploadDir() string {
	return "./uploads"
}
