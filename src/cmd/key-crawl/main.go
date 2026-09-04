package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Captain-Tom-CL/key-crawl/src/internal/routes"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

const key string = "4195282"
const version string = "3"

func main() {
	v := flag.String("version", "", "version check")
	startKey := flag.String("key", "", "start key")
	isDev := flag.Bool("dev", false, "enable development mode")
	flag.Parse()
	if *v != "" {
		fmt.Print(version)
		os.Exit(0)
	}
	if *startKey != key {
		fmt.Println("unauthorized")
		os.Exit(1)
	}
	startServer(*isDev)
}
func startServer(isDev bool) {
	e := echo.New()
	if isDev {
		e.Use(middleware.RequestLogger())
	}
	e.Use(middleware.Recover())
	routes.RegisterHealthRoutes(e)
	routes.RegisterSettingsRoutes(e)
	routes.RegisterHTMLRoutes(e)
	routes.RegisterAnalyzerRoutes(e)
	if err := e.Start(":1323"); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}
