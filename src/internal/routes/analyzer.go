package routes

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path"

	"github.com/labstack/echo/v5"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

const resultsFolder string = "data/results"

func RegisterAnalyzerRoutes(e *echo.Echo) {
	group := e.Group("analyzer")
	group.GET("", analyzeHTML)
}

func analyzeHTML(c *echo.Context) error {
	htmls, err := os.ReadDir(htmlFolder)
	if err != nil {
		return err
	}
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	success := []string{}
	for _, html := range htmls {
		file := path.Join(htmlFolder, html.Name())
		result, err := analyze(settings, file, c.Request().Context())
		if err != nil {
			log.Println(err)
			continue
		}
		data, err := json.Marshal(result)
		if err != nil {
			log.Println(err)
			continue
		}
		err = os.WriteFile(path.Join(resultsFolder, html.Name()+".json"), data, 0644)
		if err != nil {
			log.Println(err)
			continue
		}
		os.Remove(file)
		success = append(success, html.Name())
	}
	return c.JSON(200, map[string]any{"message": "Analysis complete", "success": success})
}

type AnalysisResult struct {
	JournalName     string `json:"journalName"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	KeyFindings     string `json:"keyFindings"`
	Figure          string `json:"figure"`
	Contribution    string `json:"contribution"`
	DOIURL          string `json:"doiUrl"`
	PublicationType string `json:"publicationType"`
	PublicationTime string `json:"publicationTime"`
	Authors         string `json:"authors"`
}

func getClient(settings *Settings) (*openai.Client, error) {
	client := openai.NewClient(option.WithBaseURL(settings.BaseURL), option.WithAPIKey(settings.ApiKey))
	return &client, nil
}

func analyze(settings *Settings, file string, context context.Context) (*AnalysisResult, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	content := string(data)
	client, err := getClient(settings)
	if err != nil {
		return nil, err
	}
	responseFormat := shared.NewResponseFormatJSONObjectParam()
	var result AnalysisResult
	result = AnalysisResult{}
	schema, _ := json.Marshal(result)
	resp, err := client.Chat.Completions.New(context, openai.ChatCompletionNewParams{
		Model: settings.Model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(settings.Prompt + "Return only a JSON object in exactly this form: " + string(schema)),
			openai.UserMessage(content),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &responseFormat,
		},
	})
	if err != nil {
		return nil, err
	}
	jsonString := resp.Choices[0].Message.Content
	err = json.Unmarshal([]byte(jsonString), &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
