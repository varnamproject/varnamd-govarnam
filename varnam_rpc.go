package main

import (
	"errors"
	"github.com/varnamproject/libvarnam-golang"
	"log"
)

type Args struct {
	LangCode string `json:"lang"`
	Word     string `json:"text"`
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
		handle.Learn(word)
	}
}
