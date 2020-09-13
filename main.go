package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
)

const (
	downloadPageSize = 100
)

type appConfig struct {
	Address string `koanf:"address"`

	EnableInternalApis bool   `koanf:"enable-internal-api"` // internal APIs are not exposed to public
	EnableSSL          bool   `koanf:"enable-ssl"`
	CertFilePath       string `koanf:"cert-path"`
	KeyFilePath        string `koanf:"key-file-path"`

	DownloadEnabledSchemes string        `koanf:"download-enabled-schemes"`
	SyncInterval           time.Duration `koanf:"sync-interval"`
	UpstreamURL            string        `koanf:"upstream-url"`
}

var (
	kf = koanf.New(".")

	uiDir = "ui/"

	varnamdConfig         *config // config instance used across the application
	syncDispatcherRunning bool
	startedAt             time.Time
	buildVersion          string
	buildDate             string
	maxHandleCount        int
)

// App is a singleton to share across handlers.
type App struct {
	cache Cache
	log   *log.Logger
}

// varnamd configurations
// this is populated from various command line flags
type config struct {
	upstream          string
	schemesToDownload map[string]bool
	syncInterval      time.Duration
}

func init() {
	//  Setup flags to read from user to start the application.
	// Initialize 'config' flagset.
	flagSet := flag.NewFlagSet("config", flag.ContinueOnError)
	flagSet.Usage = func() {
		log.Fatal(flagSet.FlagUsages())
	}

	// Create  config flag to read 'config.toml' from user.
	flagSet.String("config", "config.toml", "Path to the TOML configuration file")

	// Create flag for version check.
	flagSet.Bool("version", false, "Current version of the build")

	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		log.Fatalf("error parsing flags: %v", err)
	}

	// Load commandline params user given.
	if err = kf.Load(posflag.Provider(flagSet, ".", kf), nil); err != nil {
		log.Fatal(err.Error())
	}

	// Handle --version flag. Print build version, build date and die.
	if kf.Bool("version") {
		fmt.Printf("Commit: %v\nBuild: %v\n", buildVersion, buildDate)
		os.Exit(0)
	}

	// Load the config file.
	log.Printf("reading config: %s", kf.String("config"))

	if err = kf.Load(file.Provider(kf.String("config")), toml.Parser()); err != nil {
		log.Fatalf("error reading config: %v", err)
	}

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
	var (
		config appConfig
	)

	runtime.GOMAXPROCS(runtime.NumCPU())

	// Read configuration using Koanf.
	if err := kf.Unmarshal("app", &config); err != nil {
		log.Fatal(err.Error())
	}

	maxHandleCount = kf.Int("app.max-handles")
	if maxHandleCount <= 0 {
		maxHandleCount = 10
	}

	varnamdConfig = initConfig(config)
	startedAt = time.Now()

	log.Printf("varnamd %s-%s", buildVersion, buildDate)

	app := &App{
		cache: NewMemCache(),
		log:   log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile),
	}

	startSyncDispatcher()
	startDaemon(app, config)
}
