package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

func startDaemon() {
	initLanguageChannels()
	initLearnChannels()
	r := mux.NewRouter()
	r.HandleFunc("/tl/{langCode}/{word}", transliterationHandler).Methods("GET")
	r.HandleFunc("/rtl/{langCode}/{word}", reverseTransliterationHandler).Methods("GET")
	r.HandleFunc("/meta/{langCode}", metadataHandler).Methods("GET")
	r.HandleFunc("/download/{langCode}/{downloadStart}", downloadHandler).Methods("GET")
	r.HandleFunc("/learn", learnHandler).Methods("POST")
	r.HandleFunc("/languages", languagesHandler).Methods("GET")
	r.HandleFunc("/status", statusHandler).Methods("GET")
	if enableInternalApis {
		r.HandleFunc("/sync/download/{langCode}/enable", enableDownload).Methods("POST")
		r.HandleFunc("/sync/download/{langCode}/disable", disableDownload).Methods("POST")
	}

	addUI(r)

	address := fmt.Sprintf("%s:%d", host, port)
	log.Printf("Starting server at %s", address)
	if enableSSL {
		if err := http.ListenAndServeTLS(address, certFilePath, keyFilePath, recoverHandler(corsHandler(r))); err != nil {
			log.Fatalln(err)
		}
	} else {
		if err := http.ListenAndServe(address, recoverHandler(corsHandler(r))); err != nil {
			log.Fatalln(err)
		}
	}
}

func addUI(r *mux.Router) {
	if uiDir == "" {
		return
	}

	if _, err := os.Stat(uiDir); err != nil {
		log.Fatalln("UI path doesnot exist", err)
	}

	r.PathPrefix("/").Handler(http.FileServer(http.Dir(uiDir)))
}
