package routes

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/labstack/echo/v5"
)

var settings *Settings

const settingsFile string = "data/settings.json"

func RegisterSettingsRoutes(e *echo.Echo) {
	settings, _ = loadSettings()
	group := e.Group("settings")
	group.GET("", getSettings)
	group.POST("", addSettings)
}

func getSettings(c *echo.Context) error {
	return c.JSON(200, settings)
}
func addSettings(c *echo.Context) error {
	var body Settings
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(
			http.StatusBadRequest,
			"invalid request body",
		)
	}
	settings = &body
	if err := saveSettings(); err != nil {
		return err
	}
	return c.JSON(200, settings)
}

type Settings struct {
	ApiKey  string `json:"apiKey"`
	BaseURL string `json:"baseURL"`
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
}

func loadSettings() (*Settings, error) {
	if _, err := os.Stat(settingsFile); os.IsNotExist(err) {
		return &Settings{}, nil
	}
	data, err := os.ReadFile(settingsFile)
	if err != nil {
		return nil, err
	}
	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

func saveSettings() error {
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return os.WriteFile(settingsFile, data, 0644)
}
