package main

import "flag"

var (
	port           int
	learnOnly      bool
	maxHandleCount int
	learnPort      int
	host           string
)

func init() {
	flag.IntVar(&port, "p", 8080, "Run daemon in specified port")
	flag.IntVar(&learnPort, "lp", 8088, "Run learn daemon in specified port (rpc port)")
	flag.BoolVar(&learnOnly, "learn-only", false, "Run learn only daemon")
	flag.IntVar(&maxHandleCount, "max-handle-count", 10, "Maximum number of handles can be opened for each language")
	flag.StringVar(&host, "host", "", "Host for the varnam daemon server")
}

func main() {
	flag.Parse()

	startServer()
}
