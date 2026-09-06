package routes

import (
	"net/http"

	"github.com/captain-tom-cl/key-crawl/src/internal/settings"

	"github.com/labstack/echo/v5"
)

func RegisterSettingsRoutes(e *echo.Echo) {
	group := e.Group("settings")
	group.GET("", getSettings)
	group.POST("", addSettings)
}

func getSettings(c *echo.Context) error {
	snapshot, err := settings.Snapshot()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "配置加载失败，请检查或重新保存配置")
	}
	return c.JSON(200, snapshot)
}

func addSettings(c *echo.Context) error {
	body := *settings.Default()
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(
			http.StatusBadRequest,
			"invalid request body",
		)
	}
	if err := body.Validate(); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := settings.Save(body); err != nil {
		return err
	}
	return c.JSON(200, body)
}
