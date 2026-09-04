package routes

import (
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/labstack/echo/v5"
)

func init() {
	os.MkdirAll(htmlFolder, 0755)
	os.MkdirAll(resultsFolder, 0755)
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
	url = strings.ToLower(url)
	reg := regexp.MustCompile(`[^0-9a-z\.\-]+`)
	f, err := os.OpenFile(path.Join(htmlFolder, reg.ReplaceAllString(url, "-")), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
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
