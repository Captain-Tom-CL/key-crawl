package routes

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

func RegisterHealthRoutes(e *echo.Echo) {
	group := e.Group("health")
	group.GET("", getHealth)
}

func getHealth(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "healthy"})
}
