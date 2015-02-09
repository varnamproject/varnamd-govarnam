package main

import "flag"
import "fmt"
import "os"

var (
	port           int
	learnOnly      bool
	version        bool
	maxHandleCount int
	learnPort      int
	host           string
	uiDir          string
)

func init() {
	flag.IntVar(&port, "p", 8080, "Run daemon in specified port")
	flag.IntVar(&learnPort, "lp", 8088, "Run learn daemon in specified port (rpc port)")
	flag.BoolVar(&learnOnly, "learn-only", false, "Run learn only daemon")
	flag.IntVar(&maxHandleCount, "max-handle-count", 10, "Maximum number of handles can be opened for each language")
	flag.StringVar(&host, "host", "", "Host for the varnam daemon server")
	flag.StringVar(&uiDir, "ui", "", "UI directory path")
	flag.BoolVar(&version, "version", false, "Print the version and exit")
}

func main() {
	flag.Parse()
	if version {
		fmt.Println(VERSION)
		os.Exit(0)
	}
	startServer()
}
