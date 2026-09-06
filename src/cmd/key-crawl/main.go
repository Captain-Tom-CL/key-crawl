package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/captain-tom-cl/key-crawl/src/internal/routes"
	"github.com/captain-tom-cl/key-crawl/src/internal/settings"
	"github.com/captain-tom-cl/key-crawl/src/internal/storage"

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
	if !*isDev {
		restarting, err := update()
		if err != nil {
			fmt.Fprintln(os.Stderr, "自动更新检查失败，将继续启动当前版本:", err)
		} else if restarting {
			return
		}
	}

	if err := startServer(*isDev, *cleanupUpdatePath); err != nil {
		fmt.Fprintln(os.Stderr, "服务启动失败:", err)
		os.Exit(1)
	}
}

func startServer(isDev bool, cleanupUpdatePath string) error {
	if err := storage.Initialize(); err != nil {
		return fmt.Errorf("初始化数据目录失败: %w", err)
	}
	e := echo.New()
	if isDev {
		e.Use(middleware.RequestLogger())
	}
	e.Use(middleware.Recover())
	routes.RegisterHomeRoutes(e)
	routes.RegisterHealthRoutes(e)
	if err := settings.Initialize(); err != nil {
		e.Logger.Error("加载配置失败，请检查或重新保存配置", "file", settings.FilePath, "error", err)
	}
	routes.RegisterSettingsRoutes(e)
	routes.RegisterHTMLRoutes(e)
	routes.RegisterAnalyzerRoutes(e)
	routes.RegisterResultsRoutes(e)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	config := echo.StartConfig{
		Address: "127.0.0.1:1323",
		ListenerAddrFunc: func(addr net.Addr) {
			fmt.Println()
			fmt.Printf("\033[1;32m● 服务已启动\033[0m  打开 \033[1;4;94mhttp://%s\033[0m 查看说明\n", addr.String())
			// The listener is bound successfully; failed initialization or binding
			// must leave update backups intact for manual recovery.
			if cleanupUpdatePath != "" {
				go cleanupUpdateFiles(cleanupUpdatePath)
			}
		},
	}
	return config.Start(ctx, e)
}
