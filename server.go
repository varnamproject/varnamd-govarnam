package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func startDaemon(app *App) {
	initLanguageChannels()
	initLearnChannels()

	e, err := initHandlers(app)
	if err != nil {
		app.log.Fatalf("error initializing handlers: %s", err)
	}

	address := fmt.Sprintf("%s:%d", host, port)
	app.log.Printf("Listening on %s", address)

	if enableSSL {
		if err := e.StartTLS(address, certFilePath, keyFilePath); err != nil {
			app.log.Fatal(err)
		}
	} else {
		if err := e.Start(address); err != nil {
			app.log.Fatal(err)
		}
	}
}

func initHandlers(app *App) (*echo.Echo, error) {
	e := echo.New()

	e.GET("/tl/:langCode/:word", handleTransliteration)
	e.GET("/rtl/:langCode/:word", handleReverseTransliteration)
	e.GET("/meta/:langCode:", handleMetadata)
	e.GET("/download/:langCode/:downloadStart", handleDownload)
	e.POST("/learn", handlLearn)
	e.GET("/languages", handleLanguages)
	e.GET("/status", handleStatus)
	e.POST("/train", handleTrain)

	if _, err := os.Stat(filepath.Clean(uiDir)); err != nil {
		return nil, err
	}

	e.Static("/", filepath.Clean(uiDir))

	if enableInternalApis {
		e.POST("/sync/download/{langCode}/enable", handleEnableDownload)
		e.POST("/sync/download/{langCode}/disable", handleDisableDownload)
	}

	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

	// Custom middleware to set sigleton to app's context.
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("app", app)
			return next(c)
		}
	})

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodOptions},
	}))

	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:      "",
		ContentTypeNosniff: "",
		XFrameOptions:      "",
		HSTSMaxAge:         3600,
		// ContentSecurityPolicy: "default-src 'self'",
	}))

	return e, nil
}
