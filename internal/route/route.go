package route

import (
	"github.com/labstack/echo/v4"
	"uploader/internal/api"
)

func Register(e *echo.Echo) {
	g := e.Group("api")
	g.GET("/dir", api.Dir)
	g.POST("/upload", api.Upload)
}
