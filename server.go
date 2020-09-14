package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func startDaemon(app *App, cfg appConfig) {
	initLanguageChannels()
	initLearnChannels()

	e, err := initHandlers(app, cfg.EnableInternalApis)
	if err != nil {
		app.log.Fatalf("error initializing handlers: %s", err)
	}

	app.log.Printf("Listening on %s", cfg.Address)

	if cfg.EnableSSL {
		if err := e.StartTLS(cfg.Address, cfg.CertFilePath, cfg.KeyFilePath); err != nil {
			app.log.Fatal(err)
		}
	} else {
		if err := e.Start(cfg.Address); err != nil {
			app.log.Fatal(err)
		}
	}
}

func initHandlers(app *App, enableInternalApis bool) (*echo.Echo, error) {
	e := echo.New()
	e.GET("/tl/:langCode/:word", handleTransliteration)
	e.GET("/rtl/:langCode/:word", handleReverseTransliteration)
	e.GET("/meta/:langCode:", handleMetadata)
	e.GET("/download/:langCode/:downloadStart", handleDownload)
	e.POST("/learn", handleLearn)
	e.GET("/languages", handleLanguages)
	e.GET("/status", handleStatus)
	e.POST("/train", handleTrain)

	e.GET("/", handleIndex)

	e.GET("/*", echo.WrapHandler(app.fs.FileServer()))

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
