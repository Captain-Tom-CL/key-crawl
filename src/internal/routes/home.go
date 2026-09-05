package routes

import (
	"embed"
	"html/template"

	"github.com/labstack/echo/v5"
)

//go:embed templates/home.html
var homeTemplateFS embed.FS

var homePageTemplate = template.Must(template.ParseFS(homeTemplateFS, "templates/home.html"))

func RegisterHomeRoutes(e *echo.Echo) {
	e.GET("/", getHome)
}

func getHome(c *echo.Context) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	return homePageTemplate.ExecuteTemplate(c.Response(), "home.html", nil)
}
