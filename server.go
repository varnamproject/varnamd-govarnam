package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func startDaemon() {
	initLanguageChannels()
	initLearnChannels()

	e := echo.New()

	e.GET("/tl/:langCode/:word", handleTransliteration)
	e.GET("/rtl/:langCode/:word", handleReverseTransliteration)
	e.GET("/meta/:langCode:", handleMetadata)
	e.GET("/download/:langCode/:downloadStart", handleDownload)
	e.POST("/learn", handlLearn)
	e.GET("/languages", handleLanguages)
	e.GET("/status", handleStatus)

	if _, err := os.Stat(filepath.Clean(uiDir)); err != nil {
		log.Fatal("UI path doesnot exist", err)
	}

	e.Static("/", filepath.Clean(uiDir))

	if enableInternalApis {
		e.POST("/sync/download/{langCode}/enable", handleEnableDownload)
		e.POST("/sync/download/{langCode}/disable", handleDisableDownload)
	}

	address := fmt.Sprintf("%s:%d", host, port)
	log.Printf("Listening on %s", address)

	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

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

	if enableSSL {
		if err := e.StartTLS(address, certFilePath, keyFilePath); err != nil {
			log.Fatal(err)
		}
	} else {
		if err := e.Start(address); err != nil {
			log.Fatal(err)
		}
	}
}
