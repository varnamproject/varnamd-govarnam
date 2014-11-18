package main

import (
	"errors"
	"log"

	"github.com/varnamproject/libvarnam-golang"
)

type Args struct {
	LangCode string
	Word     string
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
	for _, lang := range languages {
		learnChannels[lang] = make(chan string, 100)
		handle, err := libvarnam.Init(lang)
		if err != nil {
			log.Fatal("Unable to initialize varnam for lang", lang)
		}
		go listenForWords(lang, handle)
	}
}

func listenForWords(lang string, handle *libvarnam.Varnam) {
	for word := range learnChannels[lang] {
		handle.Learn(word)
	}
}
