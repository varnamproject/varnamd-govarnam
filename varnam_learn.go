package main

import (
	"log"

	"github.com/varnamproject/libvarnam-golang"
)

var (
	learnChannels map[string]chan string
)

func initLearnChannels() {
	learnChannels = make(map[string]chan string)
	for _, scheme := range schemeDetails {
		learnChannels[scheme.Identifier] = make(chan string, 100)
		handle, err := libvarnam.Init(scheme.Identifier)

		if err != nil {
			log.Fatal("Unable to initialize varnam for lang", scheme.LangCode)
		}

		go listenForWords(scheme.Identifier, handle)
	}
}

func listenForWords(lang string, handle *libvarnam.Varnam) {
	for word := range learnChannels[lang] {
		err := handle.Learn(word)
		if err != nil {
			log.Printf("Failed to learn %s. %s\n", word, err.Error())
		}
	}
}
