package routes

import (
	"errors"
	"net/http"

	"github.com/captain-tom-cl/key-crawl/src/internal/analysis"

	"github.com/labstack/echo/v5"
)

func RegisterAnalyzerRoutes(e *echo.Echo) {
	group := e.Group("analyzer")
	group.GET("", getAnalysisStatus)
	group.POST("", startAnalysis)
}

func getAnalysisStatus(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, analysis.Status())
}

func startAnalysis(c *echo.Context) error {
	status, err := analysis.Start()
	code := http.StatusOK
	switch {
	case errors.Is(err, analysis.ErrAlreadyRunning):
		code = http.StatusConflict
	case err != nil:
		code = http.StatusInternalServerError
	case status.State == analysis.Running:
		code = http.StatusAccepted
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(code, status)
}
