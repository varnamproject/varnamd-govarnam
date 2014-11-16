package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func startServer() {
	if learnOnly {
		startLearnOnlyDaemon()
	} else {
		startDaemon()
	}
}

func startLearnOnlyDaemon() {
}

func startDaemon() {
	initLanguageChannels()
	r := mux.NewRouter()
	r.HandleFunc("/tl/{langCode}/{word}", transliterationHandler).Methods("GET")
	r.HandleFunc("/rtl/{langCode}/{word}", reverseTransliterationHandler).Methods("GET")
	r.HandleFunc("/learn", learnHandler).Methods("POST")

	address := fmt.Sprintf(":%d", port)
	log.Printf("Starting server at %s", address)
	if err := http.ListenAndServe(address, r); err != nil {
		log.Fatalln(err)
	}
}
