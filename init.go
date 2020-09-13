package main

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"
	"time"
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
