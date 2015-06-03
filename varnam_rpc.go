package main

import (
	"errors"
	"log"

	"github.com/varnamproject/libvarnam-golang"
)

type Args struct {
	LangCode string `json:"lang"`
	Word     string `json:"word"`
}

type VarnamRPC struct{}

func (v *VarnamRPC) Learn(args *Args, reply *bool) error {
	ch, ok := learnChannels[args.LangCode]
	if !ok {
		return errors.New("Unable to find language")
	}
	go func(word string) { ch <- word }(args.Word)
	return nil
}

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
