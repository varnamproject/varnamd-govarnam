package main

import "flag"

var (
	port           int
	learnOnly      bool
	maxHandleCount int
)

func init() {
	flag.IntVar(&port, "p", 8080, "Run daemon in specified port")
	flag.BoolVar(&learnOnly, "learn-only", false, "Run learn only daemon")
	flag.IntVar(&maxHandleCount, "max-handle-count", 10, "Maximum number of handles can be opened for each language")
}

func main() {
	flag.Parse()

	startServer()
}
