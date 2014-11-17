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

func getOrCreateHandler(langCode string, f func(handle *libvarnam.Varnam) (data interface{}, err error)) (data interface{}, err error) {
	ch, ok := langaugeChannels[langCode]
	if !ok {
		return nil, errors.New("Invalid Language code")
	}
	select {
	case handle := <-ch:
		data, err = f(handle)
		go func() { ch <- handle }()
	default:
		var handle *libvarnam.Varnam
		handle, err = libvarnam.Init(langCode)
		if err != nil {
			log.Println(err)
			return nil, errors.New("Unable to initialize varnam handle")
		}
		data, err = f(handle)
		go sendHandlerToChannel(langCode, handle, ch)
	}
	return
}

func transliterate(langCode string, word string) (data interface{}, err error) {
	return getOrCreateHandler(langCode, func(handle *libvarnam.Varnam) (data interface{}, err error) {
		data, err = handle.Transliterate(word)
		return
	})
}

func reveseTransliterate(langCode string, word string) (data interface{}, err error) {
	return getOrCreateHandler(langCode, func(handle *libvarnam.Varnam) (data interface{}, err error) {
		data, err = handle.ReverseTransliterate(word)
		return
	})
}

func sendHandlerToChannel(langCode string, handle *libvarnam.Varnam, ch chan *libvarnam.Varnam) {
	mutex.Lock()
	count := channelsCount[langCode]
	mutex.Unlock()
	if count == maxHandleCount {
		log.Printf("Throw away handle")
		return
	}
	select {
	case ch <- handle:
		mutex.Lock()
		count = channelsCount[langCode]
		channelsCount[langCode] = count + 1
		mutex.Unlock()
	default:
	}
}
