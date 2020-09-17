package main

import (
	"strings"

	"github.com/varnamproject/varnamd/libvarnam"
)

const defaultChanSize = 1000

var (
	learnChannels map[string]chan string
	trainChannel  map[string]chan trainArgs
)

// initChannels method will initialize learn and train channels.
func (app *App) initChannels() {
	learnChannels = make(map[string]chan string)
	trainChannel = make(map[string]chan trainArgs)

	for _, scheme := range schemeDetails {
		learnChannels[scheme.Identifier] = make(chan string, defaultChanSize)
		trainChannel[scheme.Identifier] = make(chan trainArgs, defaultChanSize)

		handle, err := libvarnam.Init(scheme.Identifier)
		if err != nil {
			app.log.Fatal("Unable to initialize varnam for lang", scheme.LangCode)
		}

		go app.listenForWords(scheme.Identifier, handle)
	}
}

func (app *App) listenForWords(lang string, handle *libvarnam.Varnam) {
	for {
		select {
		case word := <-learnChannels[lang]:
			if err := handle.Learn(strings.TrimSpace(word)); err != nil {
				app.log.Printf("Failed to learn %s. %s\n", word, err.Error())
			}
		case args := <-trainChannel[lang]:
			if err := handle.Train(strings.TrimSpace(args.Pattern), strings.TrimSpace(args.Word)); err != nil {
				app.log.Printf("error training word: %s, pattern: %s, err:%s", args.Word, args.Pattern, err.Error())
			}
		}
	}
}
