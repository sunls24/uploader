package api

import (
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

const baseDir = "./fs"

func Dir(c echo.Context) error {
	entryList, err := os.ReadDir(filepath.Join(baseDir, c.QueryParam("p")))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var dirs = make([]string, 0)
	for _, e := range entryList {
		if !e.IsDir() {
			continue
		}

		dirs = append(dirs, e.Name())
	}
	return c.JSON(http.StatusOK, dirs)
}

func Upload(c echo.Context) error {
	dir := c.QueryParam("dir")
	form, err := c.MultipartForm()
	if err != nil {
		return err
	}
	var eg errgroup.Group
	for _, fh := range form.File["files"] {
		eg.Go(func() error {
			return writeFile(fh, dir)
		})
	}
	if err = eg.Wait(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return nil
}

func writeFile(fh *multipart.FileHeader, dir string) error {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(filepath.Join(baseDir, dir, fh.Filename))
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}

	log.Info().Msgf("done: %s %s", dir, fh.Filename)
	return nil
}
