package routes

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path"
	"sync/atomic"

	"github.com/labstack/echo/v5"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

const resultsFolder string = "data/results"

var isAnalyzing atomic.Bool
var analyzerLogger = log.New(os.Stdout, "[分析] ", log.LstdFlags)

func tryBeginAnalysis() bool {
	return isAnalyzing.CompareAndSwap(false, true)
}

func finishAnalysis() {
	isAnalyzing.Store(false)
}

func RegisterAnalyzerRoutes(e *echo.Echo) {
	group := e.Group("analyzer")
	group.GET("", analyzeHTML)
}

func analyzeHTML(c *echo.Context) error {
	if !tryBeginAnalysis() {
		analyzerLogger.Println("已有分析任务正在进行，本次请求已直接返回")
		return c.JSON(200, map[string]any{"message": "Analysis already in progress"})
	}
	defer finishAnalysis()

	analyzerLogger.Println("开始扫描待分析文件")
	htmls, err := os.ReadDir(htmlFolder)
	if err != nil {
		analyzerLogger.Printf("读取待分析目录失败：%v", err)
		return err
	}
	settings, err := loadSettings()
	if err != nil {
		analyzerLogger.Printf("读取设置失败：%v", err)
		return err
	}
	analyzerLogger.Printf("发现 %d 个待处理项目", len(htmls))

	success := []string{}
	failed := 0
	for _, html := range htmls {
		if html.IsDir() {
			analyzerLogger.Printf("跳过目录：%s", html.Name())
			continue
		}

		analyzerLogger.Printf("开始处理文件：%s", html.Name())
		file := path.Join(htmlFolder, html.Name())
		result, err := analyze(settings, file, c.Request().Context())
		if err != nil {
			failed++
			analyzerLogger.Printf("文件处理失败：%s；分析错误：%v", html.Name(), err)
			continue
		}
		data, err := json.Marshal(result)
		if err != nil {
			failed++
			analyzerLogger.Printf("文件处理失败：%s；结果序列化错误：%v", html.Name(), err)
			continue
		}
		err = os.WriteFile(path.Join(resultsFolder, html.Name()+".json"), data, 0644)
		if err != nil {
			failed++
			analyzerLogger.Printf("文件处理失败：%s；保存结果错误：%v", html.Name(), err)
			continue
		}
		if err := os.Remove(file); err != nil {
			failed++
			analyzerLogger.Printf("文件处理失败：%s；删除源文件错误：%v", html.Name(), err)
			continue
		}
		success = append(success, html.Name())
		analyzerLogger.Printf("文件处理完成：%s", html.Name())
	}
	analyzerLogger.Printf("分析任务完成：成功 %d 个，失败 %d 个", len(success), failed)
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
