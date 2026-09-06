package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/captain-tom-cl/key-crawl/src/internal/settings"
	"github.com/captain-tom-cl/key-crawl/src/internal/storage"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

// Bounds one file's model requests, including rate waits and retries, not the batch.
const modelRequestTimeout = 5 * time.Minute

// A worker processes each file once. Only model requests are retried, never
// result writes or source deletion, which could cause duplicate model charges.
func processAnalysisFile(ctx context.Context, config *settings.Config, requester *modelRequester, fileName string) error {
	result, err := analyze(config, requester, fileName, ctx)
	if err != nil {
		return fmt.Errorf("分析错误: %w", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("结果序列化错误: %w", err)
	}
	resultFileName := strings.TrimSuffix(fileName, ".html") + ".json"
	if err := storage.WriteResult(resultFileName, data); err != nil {
		return fmt.Errorf("保存结果错误: %w", err)
	}
	if err := storage.RemoveHTML(fileName); err != nil {
		return fmt.Errorf("删除源文件错误: %w", err)
	}
	return nil
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

// The source URL is supplied by the application, not included in the model schema.
type storedAnalysisResult struct {
	AnalysisResult
	URL string `json:"url"`
}

func analyze(config *settings.Config, requester *modelRequester, fileName string, ctx context.Context) (*storedAnalysisResult, error) {
	url, content, err := storage.ReadSavedHTML(fileName)
	if err != nil {
		return nil, err
	}
	responseFormat := shared.NewResponseFormatJSONObjectParam()
	var result AnalysisResult
	result = AnalysisResult{}
	schema, _ := json.Marshal(result)
	resp, err := requester.complete(ctx, fileName, openai.ChatCompletionNewParams{
		Model: config.Model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(config.Prompt + "Return only a JSON object in exactly this form: " + string(schema)),
			openai.UserMessage(content),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &responseFormat,
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, errors.New("模型响应的 choices 为空")
	}
	jsonString := resp.Choices[0].Message.Content
	err = json.Unmarshal([]byte(jsonString), &result)
	if err != nil {
		return nil, err
	}
	return &storedAnalysisResult{AnalysisResult: result, URL: url}, nil
}
