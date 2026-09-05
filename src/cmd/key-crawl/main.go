package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/captain-tom-cl/key-crawl/src/internal/routes"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func main() {
	fmt.Println("应用启动中...")
	applyUpdatePath := flag.String("apply-update", "", "internal update target")
	extensionUpdatePath := flag.String("extension-update", "", "internal extension target")
	cleanupUpdatePath := flag.String("cleanup-update", "", "internal update cleanup path")
	isDev := flag.Bool("dev", false, "enable development mode")
	flag.Parse()

	if *applyUpdatePath != "" {
		if err := applyUpdate(*applyUpdatePath, *extensionUpdatePath); err != nil {
			fmt.Fprintln(os.Stderr, "应用更新失败:", err)
			os.Exit(1)
		}
		return
	}
	if !*isDev {
		if err := setApplicationWorkingDirectory(); err != nil {
			fmt.Fprintln(os.Stderr, "设置应用工作目录失败:", err)
			os.Exit(1)
		}
	}
	if *cleanupUpdatePath != "" {
		go cleanupUpdateFiles(*cleanupUpdatePath)
	}

	if !*isDev {
		restarting, err := update()
		if err != nil {
			fmt.Fprintln(os.Stderr, "自动更新检查失败，将继续启动当前版本:", err)
		} else if restarting {
			return
		}
	}

	startServer(*isDev)
}

func startServer(isDev bool) {
	if err := routes.InitializeStorage(); err != nil {
		fmt.Fprintln(os.Stderr, "初始化数据目录失败:", err)
		return
	}
	e := echo.New()
	if isDev {
		e.Use(middleware.RequestLogger())
	}
	e.Use(middleware.Recover())
	routes.RegisterHomeRoutes(e)
	routes.RegisterHealthRoutes(e)
	routes.RegisterSettingsRoutes(e)
	routes.RegisterHTMLRoutes(e)
	routes.RegisterAnalyzerRoutes(e)
	routes.RegisterResultsRoutes(e)
	fmt.Println()
	fmt.Println("\033[1;32m● 服务已启动\033[0m  打开 \033[1;4;94mhttp://localhost:1323\033[0m 查看说明")
	if err := e.Start(":1323"); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}
