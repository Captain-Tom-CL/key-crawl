package routes

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/labstack/echo/v5"
)

type resultView struct {
	AnalysisResult
	URL      string `json:"url"`
	FileName string `json:"fileName"`
}

type resultsPageData struct {
	Results     []resultView
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
	results, warnings, err := loadResultViews()
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

func loadResultViews() ([]resultView, []string, error) {
	entries, err := os.ReadDir(resultsFolder)
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	results := make([]resultView, 0, len(entries))
	warnings := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}

		result, err := readResultView(entry.Name())
		if err != nil {
			warnings = append(warnings, entry.Name()+"："+err.Error())
			continue
		}
		results = append(results, result)
	}

	return results, warnings, nil
}

func readResultView(fileName string) (resultView, error) {
	data, err := os.ReadFile(filepath.Join(resultsFolder, fileName))
	if err != nil {
		return resultView{}, err
	}

	var result AnalysisResult
	if err := json.Unmarshal(data, &result); err != nil {
		return resultView{}, err
	}

	encodedURL := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	decodedURL, err := base64.URLEncoding.DecodeString(encodedURL)
	if err != nil {
		return resultView{}, err
	}

	return resultView{
		AnalysisResult: result,
		URL:            string(decodedURL),
		FileName:       fileName,
	}, nil
}
