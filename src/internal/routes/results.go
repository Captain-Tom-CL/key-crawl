package routes

import (
	"embed"
	"encoding/json"
	"html/template"

	"github.com/captain-tom-cl/key-crawl/src/internal/analysis"

	"github.com/labstack/echo/v5"
)

type resultsPageData struct {
	Results     []analysis.ResultFile
	Warnings    []string
	ResultsJSON template.JS
}

//go:embed templates/results.html
var resultsTemplateFS embed.FS

var resultsPageTemplate = template.Must(template.ParseFS(resultsTemplateFS, "templates/results.html"))

func RegisterResultsRoutes(e *echo.Echo) {
	group := e.Group("results")
	group.GET("", getResults)
}

func getResults(c *echo.Context) error {
	results, warnings, err := analysis.LoadResults()
	if err != nil {
		return err
	}

	encodedResults, err := json.Marshal(results)
	if err != nil {
		return err
	}

	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	return resultsPageTemplate.ExecuteTemplate(c.Response(), "results.html", resultsPageData{
		Results:     results,
		Warnings:    warnings,
		ResultsJSON: template.JS(encodedResults),
	})
}
