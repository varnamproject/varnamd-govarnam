package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"runtime"
	"strings"
	"time"
)

var (
	port                   int
	version                bool
	maxHandleCount         int
	host                   string
	uiDir                  string
	enableInternalApis     bool // internal APIs are not exposed to public
	enableSSL              bool
	certFilePath           string
	keyFilePath            string
	logToFile              bool    // logs will be written to file when true
	varnamdConfig          *config // config instance used across the application
	startedAt              time.Time
	downloadEnabledSchemes string // comma separated list of scheme identifier for which download will be performed
	syncIntervalInSecs     int
	upstreamURL            string
	syncDispatcherRunning  bool
)

// varnamd configurations
// this is populated from various command line flags
type config struct {
	upstream          string
	schemesToDownload map[string]bool
	syncInterval      time.Duration
}

func initConfig() *config {
	toDownload := make(map[string]bool)
	schemes := strings.Split(downloadEnabledSchemes, ",")

	for _, scheme := range schemes {
		s := strings.TrimSpace(scheme)

		if s != "" {
			if !isValidSchemeIdentifier(s) {
				panic(fmt.Sprintf("%s is not a valid libvarnam supported scheme", s))
			}

			toDownload[s] = true
		}
	}

	return &config{upstream: upstreamURL, schemesToDownload: toDownload,
		syncInterval: time.Duration(syncIntervalInSecs)}
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

func getLogsDir() string {
	d := getConfigDir()
	logsDir := path.Join(d, "logs")

	if err := os.MkdirAll(logsDir, 0750); err != nil {
		panic(err)
	}

	return logsDir
}

func redirectLogToFile() {
	year, month, day := time.Now().Date()
	logfile := path.Join(getLogsDir(), fmt.Sprintf("%d-%d-%d.log", year, month, day))

	f, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		panic(err)
	}

	log.SetOutput(f)
}

func init() {
	flag.IntVar(&port, "p", 8080, "Run daemon in specified port")
	flag.IntVar(&maxHandleCount, "max-handle-count", 10, "Maximum number of handles can be opened for each language")
	flag.StringVar(&host, "host", "localhost", "Host for the varnam daemon server")
	flag.StringVar(&uiDir, "ui", "ui", "UI directory path")
	flag.BoolVar(&enableInternalApis, "enable-internal-apis", false, "Enable internal APIs")
	flag.BoolVar(&enableSSL, "enable-ssl", false, "Enables SSL")
	flag.StringVar(&certFilePath, "cert-file-path", "", "Certificate file path")
	flag.StringVar(&keyFilePath, "key-file-path", "", "Key file path")
	flag.StringVar(&upstreamURL, "upstream", "https://api.varnamproject.com", "Provide an upstream server")
	flag.StringVar(&downloadEnabledSchemes, "enable-download", "", "Comma separated language identifier for which varnamd will download words from upstream")
	flag.IntVar(&syncIntervalInSecs, "sync-interval", 30, "Download interval in seconds")
	flag.BoolVar(&logToFile, "log-to-file", true, "If true, logs will be written to a file")
	flag.BoolVar(&version, "version", false, "Print the version and exit")
}

func syncRequired() bool {
	return len(varnamdConfig.schemesToDownload) > 0
}

// Starts the sync process only if it is not running
func startSyncDispatcher() {
	if syncRequired() && !syncDispatcherRunning {
		sync := newSyncDispatcher(varnamdConfig.syncInterval * time.Second)
		sync.start()
		sync.runNow() // run one round of sync immediatly rather than waiting for the next interval to occur

		syncDispatcherRunning = true
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.Parse()

	varnamdConfig = initConfig()
	startedAt = time.Now()

	if version {
		fmt.Println(varnamdVersion)
		os.Exit(0)
	}

	if logToFile {
		redirectLogToFile()
	}

	log.Printf("varnamd %s", varnamdVersion)

	startSyncDispatcher()
	startDaemon()
}
