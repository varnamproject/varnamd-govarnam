package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/knadh/stuffbin"
)

func initConfig(cfg appConfig) *config {
	toDownload := make(map[string]bool)
	schemes := strings.Split(cfg.DownloadEnabledSchemes, ",")

	for _, scheme := range schemes {
		s := strings.TrimSpace(scheme)

		if s != "" {
			if !isValidSchemeIdentifier(s) {
				panic(fmt.Sprintf("%s is not a valid libvarnam supported scheme", s))
			}

			toDownload[s] = true
		}
	}

	return &config{upstream: cfg.UpstreamURL, schemesToDownload: toDownload,
		syncInterval: time.Duration(cfg.SyncInterval)}
}

func (c *config) setDownloadStatus(langCode string, status bool) error {
	if !isValidSchemeIdentifier(langCode) {
		return fmt.Errorf("%s is not a valid libvarnam supported scheme", langCode)
	}

	c.schemesToDownload[langCode] = status

	if status {
		// when varnamd was started without any langcodes to sync, the dispatcher won't be running
		// in that case, we need to start the dispatcher since we have a new lang code to download now
		startSyncDispatcher()
	}

	return nil
}

func getConfigDir() string {
	if runtime.GOOS == "windows" {
		return path.Join(os.Getenv("localappdata"), ".varnamd")
	}

	return path.Join(os.Getenv("HOME"), ".varnamd")
}

// initVFS initializes the stuffbin virtual FileSystem to provide
// access to bunded static assets to the app.
func initVFS() (stuffbin.FileSystem, error) {
	files := []string{
		"ui:/",
	}

	binPath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	fs, err := stuffbin.UnStuff(binPath)
	if err != nil {
		log.Printf("unable to initialize embedded filesystem: %v", err)
		log.Printf("using local filesystem")

		fs, err = stuffbin.NewLocalFS("/", files...)
		if err != nil {
			return nil, err
		}
	}

	return fs, nil
}

func initAppConfig() (appConfig, error) {
	var config appConfig
	// Read configuration using Koanf.
	if err := kf.Unmarshal("app", &config); err != nil {
		return config, err
	}

	// If address is empty run on localhost port 8080.
	if config.Address == "" {
		config.Address = fmt.Sprintf("%s:%d", kf.String("host"), kf.Int("p"))
	}

	if config.EnableSSL && (config.CertFilePath == "" || config.KeyFilePath == "") {
		config.EnableSSL = false
	}

	if config.SyncInterval < 1*time.Second {
		config.SyncInterval = 30 * time.Second
	}

	if config.UpstreamURL == "" {
		config.UpstreamURL = "https://api.varnamproject.com"
	}

	return config, nil
}
