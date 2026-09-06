package routes

import (
	"errors"
	"net/http"

	"github.com/captain-tom-cl/key-crawl/src/internal/storage"

	"github.com/labstack/echo/v5"
)

func RegisterHTMLRoutes(e *echo.Echo) {
	group := e.Group("html")
	group.POST("", saveHTML)
}

func saveHTML(c *echo.Context) error {
	request := c.Request()
	body := http.MaxBytesReader(c.Response(), request.Body, storage.MaxHTMLBytes)
	defer body.Close()
	err := storage.SaveHTML(request.Header.Get("x-url"), body, request.ContentLength)
	var sizeErr *http.MaxBytesError
	switch {
	case errors.Is(err, storage.ErrSourceURLRequired):
		return echo.NewHTTPError(http.StatusBadRequest, "x-url header is required")
	case errors.Is(err, storage.ErrSourceURLTooLong):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, storage.ErrHTMLTooLarge), errors.As(err, &sizeErr):
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, storage.ErrHTMLTooLarge.Error())
	case err != nil:
		return err
	}
	return c.NoContent(http.StatusCreated)
}
