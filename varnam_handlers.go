package main

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/varnamproject/libvarnam-golang"
)

var (
	languages        = []string{"ml", "hi"} // Need to replace this when libvarnam has getLanguage function
	langaugeChannels map[string]chan *libvarnam.Varnam
	channelsCount    map[string]int
	mutex            *sync.Mutex
)

func initLanguageChannels() {
	langaugeChannels = make(map[string]chan *libvarnam.Varnam)
	channelsCount = make(map[string]int)
	mutex = &sync.Mutex{}
	for _, lang := range languages {
		langaugeChannels[lang] = make(chan *libvarnam.Varnam, maxHandleCount)
		channelsCount[lang] = maxHandleCount
		for i := 0; i < maxHandleCount; i++ {
			handle, _ := libvarnam.Init(lang)
			langaugeChannels[lang] <- handle
		}
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
	case <-time.After(800 * time.Millisecond):
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
		handle.Destroy()
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
