package routes

import (
	"encoding/base64"
	"os"
	"path"
	"strings"

	"github.com/labstack/echo/v5"
)

func InitializeStorage() error {
	if err := os.MkdirAll(htmlFolder, 0755); err != nil {
		return err
	}
	return os.MkdirAll(resultsFolder, 0755)
}

func RegisterHTMLRoutes(e *echo.Echo) {
	group := e.Group("html")
	group.POST("", saveHTML)
}

const htmlFolder = "data/html"

func saveHTML(c *echo.Context) error {
	url := c.Request().Header.Get("x-url")
	url = strings.TrimSpace(url)
	if url == "" {
		return echo.NewHTTPError(400, "x-url header is required")
	}
	url = base64.URLEncoding.EncodeToString([]byte(url))
	f, err := os.OpenFile(path.Join(htmlFolder, url), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	body := c.Request().Body
	defer body.Close()
	_, err = f.ReadFrom(body)
	if err != nil {
		return err
	}
	return c.NoContent(201)
}
