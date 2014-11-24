package main

import (
	"errors"
	"log"

	"github.com/varnamproject/libvarnam-golang"
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
		learnChannels[scheme.LangCode] = make(chan string, 100)
		handle, err := libvarnam.Init(scheme.LangCode)
		if err != nil {
			log.Fatal("Unable to initialize varnam for lang", scheme.LangCode)
		}
		go listenForWords(scheme.LangCode, handle)
	}
}

func listenForWords(lang string, handle *libvarnam.Varnam) {
	for word := range learnChannels[lang] {
		handle.Learn(word)
	}
}
