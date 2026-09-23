package internal

import (
	"context"
	"errors"
	"fmt"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"uploader/config"
	"uploader/internal/server"
	"uploader/web"
)

type App struct {
}

func NewApp() App {
	return App{}
}

func (App) init() {
	log.Logger = log.Logger.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "06-01-02 15:04:05"})
}

func (app App) Run() error {
	app.init()

	e := echo.New()
	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		DisablePrintStack: true,
	}))
	defaultHTTPErrorHandler := echo.DefaultHTTPErrorHandler(false)
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		defaultHTTPErrorHandler(c, err)
		var httpErr *echo.HTTPError
		if !errors.As(err, &httpErr) || httpErr.Unwrap() != nil || httpErr.StatusCode() >= 500 {
			log.Err(err).Send()
		}
	}

	s, err := server.New()
	if err != nil {
		return err
	}
	defer s.Close()
	s.Register(e)
	e.StaticFS("/", echo.MustSubFS(web.FS, "dist"))
	cfg := config.MustNew()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result := make(chan error, 1)
	httpServer := &http.Server{Addr: fmt.Sprintf("%s:%s", cfg.Host, cfg.Port), Handler: e}
	go func() { result <- httpServer.ListenAndServe() }()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdown); err != nil {
			if closeErr := httpServer.Close(); closeErr != nil {
				return errors.Join(err, closeErr)
			}
		}
		err := <-result
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
