package main

import (
	"errors"
	"log"
	"sync"

	"github.com/varnamproject/libvarnam-golang"
)

var (
	languages        = []string{"ml", "hi"}
	langaugeChannels = make(map[string]chan *libvarnam.Varnam)
	channelsCount    = make(map[string]int)
	mutex            = &sync.Mutex{}
)

func initLanguageChannels() {
	for _, lang := range languages {
		langaugeChannels[lang] = make(chan *libvarnam.Varnam, maxHandleCount)
		channelsCount[lang] = 0
	}
}

func transliterate(langCode string, word string) (words []string, err error) {
	ch, ok := langaugeChannels[langCode]
	if !ok {
		return nil, errors.New("Invalid Language code")
	}
	select {
	case handle := <-ch:
		words, err = handle.Transliterate(word)
		go func() { ch <- handle }()
	default:
		var handle *libvarnam.Varnam
		handle, err = libvarnam.Init(langCode)
		words, err = handle.Transliterate(word)
		go sendHandlerToChannel(langCode, handle, ch)
	}
	return
}

func reveseTransliterate(langCode string, word string) (result string, err error) {
	ch, ok := langaugeChannels[langCode]
	if !ok {
		return "", errors.New("Invalid Language code")
	}
	select {
	case handle := <-ch:
		result, err = handle.ReverseTransliterate(word)
		go func() { ch <- handle }()
	default:
		var handle *libvarnam.Varnam
		handle, err = libvarnam.Init(langCode)
		result, err = handle.ReverseTransliterate(word)
		go sendHandlerToChannel(langCode, handle, ch)
	}
	return
}

func sendHandlerToChannel(langCode string, handle *libvarnam.Varnam, ch chan *libvarnam.Varnam) {
	if channelsCount[langCode] == maxHandleCount {
		log.Printf("Throw away handle")
		return
	}
	select {
	case ch <- handle:
		mutex.Lock()
		count := channelsCount[langCode]
		channelsCount[langCode] = count + 1
		mutex.Unlock()
	default:
	}
}
