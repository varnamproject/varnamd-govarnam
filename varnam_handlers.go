package main

import (
	"database/sql"
	"errors"
	"github.com/golang/groupcache"
	_ "github.com/mattn/go-sqlite3"
	"github.com/varnamproject/libvarnam-golang"
	"log"
	"sync"
	"time"
)

type word struct {
	Id         int    `json:"id"`
	Confidence int    `json:"confidence"`
	Word       string `json:"word"`
}

var (
	languageChannels map[string]chan *libvarnam.Varnam
	channelsCount    map[string]int
	mutex            *sync.Mutex
	once             sync.Once
	schemeDetails    = libvarnam.GetAllSchemeDetails()
	peers            = groupcache.NewHTTPPool("http://localhost")
	cacheGroups      = make(map[string]*groupcache.Group)
)

func initLanguageChannels() {
	languageChannels = make(map[string]chan *libvarnam.Varnam)
	channelsCount = make(map[string]int)
	mutex = &sync.Mutex{}
	for _, scheme := range schemeDetails {
		languageChannels[scheme.Identifier] = make(chan *libvarnam.Varnam, maxHandleCount)
		channelsCount[scheme.Identifier] = maxHandleCount
		for i := 0; i < maxHandleCount; i++ {
			handle, err := libvarnam.Init(scheme.Identifier)
			if err != nil {
				panic("Unable to init varnam for language " + scheme.LangCode + "." + err.Error())
			}
			languageChannels[scheme.Identifier] <- handle
		}
	}
}

func getOrCreateHandler(schemeIdentifier string, f func(handle *libvarnam.Varnam) (data interface{}, err error)) (data interface{}, err error) {
	ch, ok := languageChannels[schemeIdentifier]
	if !ok {
		return nil, errors.New("Invalid scheme identifier")
	}
	select {
	case handle := <-ch:
		data, err = f(handle)
		go func() { ch <- handle }()
	case <-time.After(800 * time.Millisecond):
		var handle *libvarnam.Varnam
		handle, err = libvarnam.Init(schemeIdentifier)
		if err != nil {
			log.Println(err)
			return nil, errors.New("Unable to initialize varnam handle")
		}
		data, err = f(handle)
		go sendHandlerToChannel(schemeIdentifier, handle, ch)
	}
	return
}

func transliterate(schemeIdentifier string, word string) (data interface{}, err error) {
	return getOrCreateHandler(schemeIdentifier, func(handle *libvarnam.Varnam) (data interface{}, err error) {
		data, err = handle.Transliterate(word)
		return
	})
}

func getWords(schemeIdentifier string, downloadStart int) ([]*word, error) {
	filepath, _ := getOrCreateHandler(schemeIdentifier, func(handle *libvarnam.Varnam) (data interface{}, err error) {
		return handle.GetSuggestionsFilePath(), nil
	})

	db, err := sql.Open("sqlite3", filepath.(string))
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query("select id, word, confidence from words limit 5000 offset ?;", downloadStart)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var words []*word
	for rows.Next() {
		var id, confidence int
		var _word string
		rows.Scan(&id, &_word, &confidence)
		words = append(words, &word{Id: id, Confidence: confidence, Word: _word})
	}

	return words, nil
}

func reveseTransliterate(schemeIdentifier string, word string) (data interface{}, err error) {
	return getOrCreateHandler(schemeIdentifier, func(handle *libvarnam.Varnam) (data interface{}, err error) {
		data, err = handle.ReverseTransliterate(word)
		return
	})
}

func sendHandlerToChannel(schemeIdentifier string, handle *libvarnam.Varnam, ch chan *libvarnam.Varnam) {
	mutex.Lock()
	count := channelsCount[schemeIdentifier]
	mutex.Unlock()
	if count == maxHandleCount {
		log.Printf("Throw away handle")
		handle.Destroy()
		return
	}
	select {
	case ch <- handle:
		mutex.Lock()
		count = channelsCount[schemeIdentifier]
		channelsCount[schemeIdentifier] = count + 1
		mutex.Unlock()
	default:
	}
}
