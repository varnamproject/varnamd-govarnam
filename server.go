package main

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func startDaemon(app *App, cfg appConfig) {
	initLanguageChannels()
	app.initChannels()

	e := initHandlers(app, cfg.EnableInternalApis)

	app.log.Printf("🚀 starting varnamd\nListening on %s", cfg.Address)

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

func initHandlers(app *App, enableInternalApis bool) *echo.Echo {
	e := echo.New()
	e.GET("/tl/:langCode/:word", handleTransliteration)
	e.GET("/rtl/:langCode/:word", handleReverseTransliteration)
	e.GET("/atl/:langCode/:word", handleAdvancedTransliteration)
	// e.GET("/meta/:langCode:", handleMetadata)
	// e.GET("/download/:langCode/:downloadStart", handleDownload)
	e.GET("/languages", handleLanguages)
	e.GET("/languages/:langCode/download", handleLanguageDownload)
	e.GET("/packs", handlePacks)
	e.GET("/packs/:langCode", handlePacks)
	e.GET("/packs/:langCode/:packIdentifier", handlePackInfo)
	e.GET("/packs/:langCode/:packIdentifier/:packPageIdentifier", handlePackPageInfo)
	e.GET("/packs/:langCode/:packIdentifier/:packPageIdentifier/download", handlePacksDownload)
	e.GET("/status", handleStatus)

	e.GET("/schemes/:schemeID", handleSchemeInfo)
	e.GET("/schemes/:schemeID/definitions", handleSchemeDefinitions)
	e.GET("/schemes/:schemeID/definitions/:letter", handleSchemeLetterDefinitions)

	e.GET("/", handleIndex)

	e.GET("/*", echo.WrapHandler(app.fs.FileServer()))

	if enableInternalApis {
		e.POST("/sync/download/:langCode/enable", handleEnableDownload)
		e.POST("/sync/download/:langCode/disable", handleDisableDownload)

		e.POST("/learn", authUser(handleLearn))
		e.POST("/learn/upload/:langCode", authUser(handleLearnFileUpload))
		e.POST("/train/:langCode", authUser(handleTrain))
		e.POST("/train/bulk/:langCode", authUser(handleTrainBulk))
		e.POST("/delete", authUser(handleDelete))
		e.POST("/packs/download", handlePackDownloadRequest)
	}

	e.Use(middleware.Recover())

	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:    true,
		LogURI:       true,
		LogLatency:   true,
		LogRemoteIP:  true,
		LogReferer:   true,
		LogUserAgent: true,
		LogError:     true,
		BeforeNextFunc: func(c echo.Context) {
			c.Set("customValueFromContext", 42)
		},
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			remoteIpMasked := md5.Sum([]byte(c.RealIP()))
			fmt.Printf(
				"[%v] status: %v, latency_human: %s, referer: %v, remote_ip: %x, error: %v, user_agent: %s, uri: %v\n",
				time.Now().Format(time.RFC3339Nano), v.Status, v.Latency.String(), v.Referer, remoteIpMasked, v.Error, v.UserAgent, v.URI,
			)
			return nil
		},
	}))

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

	// rate limit requests per second (prevent handler exhaustion)
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(20)))

	return e
}

// authUser as a separate method to apply this middleware only for selected endpoints.
func authUser(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		var app = c.Get("app").(*App)

		if authEnabled {
			auth := strings.Split(c.Request().Header.Get("Authorization"), " ")
			if len(auth) < 2 {
				app.log.Printf("authorization header not found")
				return echo.NewHTTPError(http.StatusUnauthorized, "authorization header not found")
			}

			if strings.ToLower(auth[0]) != "basic" {
				app.log.Printf("authorization header not found")
				return echo.NewHTTPError(http.StatusUnauthorized, "authorization details not found")
			}

			creds, err := base64.StdEncoding.DecodeString(auth[1])
			if err != nil {
				app.log.Printf("error decoding auth headers, error: %s", err.Error())
				return echo.NewHTTPError(http.StatusUnauthorized, "authorization failed, failed to decode authstring")
			}

			authCreds := strings.Split(string(creds), ":")

			user, ok := users[strings.TrimSpace(authCreds[0])]
			if !ok {
				app.log.Printf("user not found")
				return echo.NewHTTPError(http.StatusUnauthorized, "authorization failed, user not found")
			}

			if user["password"] != strings.TrimSpace(authCreds[1]) {
				app.log.Printf("password mismatch")
				return echo.NewHTTPError(http.StatusUnauthorized, "authorization failed, password mismatch")
			}
		}

		return next(c)
	}
}
