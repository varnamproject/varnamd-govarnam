package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"runtime"
	"time"
)

var (
	port               int
	learnOnly          bool
	version            bool
	maxHandleCount     int
	learnPort          int
	host               string
	uiDir              string
	enableInternalApis bool    // internal APIs are not exposed to public
	syncWords          bool    // when true, sync won't be performed. Useful when running on a top level server where no upstream can be configured
	varnamdConfig      *config // config instance used across the application
)

// varnamd configurations
// usually resides in $HOME/.varnamd/config on POSIX and APPDATA/.varnamd/config on Windows
type config struct {
	Upstream           string          `json:"upstream"`
	SchemesToSync      map[string]bool `json:"schemesToSync"`
	SyncIntervalInSecs time.Duration   `json:syncIntervalInSecs`
}

func initDefaultConfig() *config {
	c := &config{}
	c.setDefaultsForBlankValues()
	return c
}

func (c *config) setDefaultsForBlankValues() {
	if c.Upstream == "" {
		c.Upstream = "http://api.varnamproject.com"
	}
	if c.SchemesToSync == nil {
		c.SchemesToSync = make(map[string]bool)
	}
	if c.SyncIntervalInSecs == 0 {
		c.SyncIntervalInSecs = 30
	}
}

func getConfigDir() string {
	if runtime.GOOS == "windows" {
		return path.Join(os.Getenv("localappdata"), ".varnamd")
	} else {
		return path.Join(os.Getenv("HOME"), ".varnamd")
	}
}

func getConfigFilePath() string {
	configDir := getConfigDir()
	configFilePath := path.Join(configDir, "config.json")
	return configFilePath
}

func loadConfigFromFile() *config {
	configFilePath := getConfigFilePath()
	configFile, err := os.Open(configFilePath)
	if err != nil {
		return initDefaultConfig()
	}
	defer configFile.Close()

	jsonDecoder := json.NewDecoder(configFile)
	var c config
	err = jsonDecoder.Decode(&c)
	if err != nil {
		log.Printf("%s is malformed. Using default config instead\n", configFilePath)
		return initDefaultConfig()
	}

	c.setDefaultsForBlankValues()
	return &c
}

func (c *config) setSyncStatus(langCode string, status bool) {
	c.SchemesToSync[langCode] = status
}

func (c *config) save() error {
	configFilePath := getConfigFilePath()
	err := os.MkdirAll(path.Dir(configFilePath), 0777)
	if err != nil {
		return err
	}

	configFile, err := os.Create(configFilePath)
	if err != nil {
		return err
	}
	defer configFile.Close()

	b, err := json.MarshalIndent(c, "", "\t")
	if err != nil {
		return err
	}

	_, err = configFile.Write(b)
	if err != nil {
		return err
	}

	return nil
}

func init() {
	flag.IntVar(&port, "p", 8080, "Run daemon in specified port")
	flag.IntVar(&learnPort, "lp", 8088, "Run learn daemon in specified port (rpc port)")
	flag.BoolVar(&learnOnly, "learn-only", false, "Run learn only daemon")
	flag.IntVar(&maxHandleCount, "max-handle-count", 10, "Maximum number of handles can be opened for each language")
	flag.StringVar(&host, "host", "", "Host for the varnam daemon server")
	flag.StringVar(&uiDir, "ui", "", "UI directory path")
	flag.BoolVar(&enableInternalApis, "enable-internal-apis", false, "Enable internal APIs")
	flag.BoolVar(&syncWords, "sync-words", true, "Enable/Disable word synchronization")
	flag.BoolVar(&version, "version", false, "Print the version and exit")
	varnamdConfig = loadConfigFromFile()
}

func main() {
	flag.Parse()
	if version {
		fmt.Println(VERSION)
		os.Exit(0)
	}
	if syncWords {
		sync := newSyncDispatcher(varnamdConfig.SyncIntervalInSecs * time.Second)
		sync.start()
	}
	startServer()
}
