# 版本变量配置说明

打包脚本会在编译时注入版本信息到二进制文件中。为了使这个功能正常工作，需要在 `main.go` 中添加相应的变量。

## 需要添加的代码

在 `main.go` 文件顶部添加以下全局变量：

```go
package main

import (
    // ... 现有的 imports
)

// 版本信息（在编译时通过 -ldflags 注入）
var (
    Version   = "dev"      // 版本号，打包时会被替换
    BuildDate = "unknown"  // 构建日期，打包时会被替换
)

func main() {
    // ... 现有代码
}
```

## 如何使用版本信息

### 1. 添加 --version 命令行参数

```go
import (
    "flag"
    "fmt"
    "os"
)

var (
    Version   = "dev"
    BuildDate = "unknown"
    showVersion = flag.Bool("version", false, "显示版本信息")
)

func main() {
    flag.Parse()

    if *showVersion {
        fmt.Printf("EzPay Version: %s\n", Version)
        fmt.Printf("Build Date: %s\n", BuildDate)
        os.Exit(0)
    }

    // ... 其余代码
}
```

### 2. 在启动日志中显示

```go
func main() {
    log.Printf("Starting EzPay v%s (built on %s)", Version, BuildDate)
    // ... 其余代码
}
```

### 3. 在 Web 界面显示

可以通过 API 端点返回版本信息：

```go
func versionHandler(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "version":    Version,
        "build_date": BuildDate,
    })
}

// 在路由中注册
router.GET("/api/version", versionHandler)
```

## 编译时的工作原理

打包脚本使用 Go 的 `-ldflags` 参数在编译时替换这些变量的值：

```bash
go build -ldflags="-X main.Version=1.0.0 -X 'main.BuildDate=2024-01-30 12:00:00 UTC'" -o ezpay .
```

这样：
- `main.Version` 会被设置为 `1.0.0`
- `main.BuildDate` 会被设置为 `2024-01-30 12:00:00 UTC`

## 验证

编译后可以运行以下命令验证：

```bash
# 如果实现了 --version 参数
./ezpay --version

# 或者使用 strings 命令查找
strings ezpay | grep -A1 "Version\|BuildDate"
```

## 注意事项

1. **变量名必须是导出的（首字母大写）**: `Version` 而不是 `version`
2. **包名必须匹配**: 如果你的 main 包在子目录，需要使用完整路径，如 `-X github.com/user/ezpay/cmd.Version=1.0.0`
3. **带空格的值需要引号**: `-X 'main.BuildDate=2024-01-30 12:00:00 UTC'`

## 可选：添加更多构建信息

你还可以添加更多有用的构建信息：

```go
var (
    Version    = "dev"
    BuildDate  = "unknown"
    GitCommit  = "none"       // Git 提交哈希
    GitBranch  = "unknown"    // Git 分支
    GoVersion  = "unknown"    // Go 编译器版本
)
```

对应的构建命令：

```bash
go build -ldflags="\
    -X main.Version=${VERSION} \
    -X 'main.BuildDate=$(date -u '+%Y-%m-%d %H:%M:%S UTC')' \
    -X main.GitCommit=$(git rev-parse --short HEAD) \
    -X main.GitBranch=$(git rev-parse --abbrev-ref HEAD) \
    -X main.GoVersion=$(go version | awk '{print $3}')" \
    -o ezpay .
```

## 示例实现

完整的版本信息显示示例：

```go
package main

import (
    "flag"
    "fmt"
    "os"
    "runtime"
)

var (
    Version   = "dev"
    BuildDate = "unknown"
    GitCommit = "none"

    showVersion = flag.Bool("version", false, "显示版本信息")
)

func printVersion() {
    fmt.Printf("EzPay Payment Gateway\n")
    fmt.Printf("  Version:     %s\n", Version)
    fmt.Printf("  Build Date:  %s\n", BuildDate)
    fmt.Printf("  Git Commit:  %s\n", GitCommit)
    fmt.Printf("  Go Version:  %s\n", runtime.Version())
    fmt.Printf("  OS/Arch:     %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

func main() {
    flag.Parse()

    if *showVersion {
        printVersion()
        os.Exit(0)
    }

    // 启动时显示版本
    log.Printf("Starting EzPay v%s", Version)

    // ... 其余代码
}
```
