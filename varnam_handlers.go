package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/golang/groupcache"
	_ "github.com/mattn/go-sqlite3"
	"github.com/varnamproject/govarnam/govarnamgo"
)

type word struct {
	ID         int    `json:"id"`
	Confidence int    `json:"confidence"`
	Word       string `json:"word"`
}

var (
	languageChannels    map[string]chan *govarnamgo.VarnamHandle
	channelsCount       map[string]int
	mutex               *sync.Mutex
	once                sync.Once
	schemeDetails, errB = govarnamgo.GetAllSchemeDetails()
	cacheGroups         = make(map[string]*groupcache.Group)
	// peers            = groupcache.NewHTTPPool("http://localhost")
)

func isValidSchemeIdentifier(id string) bool {
	for _, scheme := range schemeDetails {
		if scheme.Identifier == id {
			return true
		}
	}

	return false
}

func getMaxHandleCount(schemeIdentifier string) int {
	if val, ok := maxHandleCounts[schemeIdentifier]; ok {
		return val
	} else {
		return maxHandleCounts["default"]
	}
}

func initLanguageChannels() {
	languageChannels = make(map[string]chan *govarnamgo.VarnamHandle)
	channelsCount = make(map[string]int)
	mutex = &sync.Mutex{}

	for _, scheme := range schemeDetails {
		maxHandleCount := getMaxHandleCount(scheme.Identifier)
		languageChannels[scheme.Identifier] = make(chan *govarnamgo.VarnamHandle, maxHandleCount)
		channelsCount[scheme.Identifier] = maxHandleCount

		for i := 0; i < maxHandleCount; i++ {
			handle, err := govarnamgo.InitFromID(scheme.Identifier)
			if err != nil {
				panic("Unable to init varnam for language" + scheme.LangCode + ". " + err.Error())
			}

			languageChannels[scheme.Identifier] <- handle
		}
	}
}

func getOrCreateHandler(schemeIdentifier string, f func(handle *govarnamgo.VarnamHandle) (data interface{}, err error)) (data interface{}, err error) {
	ch, ok := languageChannels[schemeIdentifier]
	if !ok {
		return nil, errors.New("invalid scheme identifier")
	}

	select {
	case handle := <-ch:
		data, err = f(handle)

		go func() { ch <- handle }()
	case <-time.After(800 * time.Millisecond):
		var handle *govarnamgo.VarnamHandle

		handle, err = govarnamgo.InitFromID(schemeIdentifier)
		if err != nil {
			log.Println(err)
			return nil, errors.New("unable to initialize varnam handle")
		}

		data, err = f(handle)
		if err != nil {
			log.Println(err)
			return nil, errors.New("unable to complete varnam handle")
		}

		go sendHandlerToChannel(schemeIdentifier, handle, ch)
	}

	return
}

func transliterate(c context.Context, schemeIdentifier string, word string) (interface{}, error) {
	return getOrCreateHandler(schemeIdentifier, func(handle *govarnamgo.VarnamHandle) (data interface{}, err error) {
		return handle.Transliterate(c, word)
	})
}

func transliterateAdvanced(c context.Context, schemeIdentifier string, word string) (interface{}, error) {
	return getOrCreateHandler(schemeIdentifier, func(handle *govarnamgo.VarnamHandle) (data interface{}, err error) {
		return handle.TransliterateAdvanced(c, word)
	})
}

// func trainwords(schemeIdentifier, word, pattern string) (interface{}, error) {
// 	return getOrCreateHandler(schemeIdentifier, func(handle *libvarnam.Varnam) (data interface{}, err error) {
// 		return handle.Train(pattern,word)
// 	})
// }

// func getWords(schemeIdentifier string, downloadStart int) ([]*word, error) {
// 	filepath, _ := getOrCreateHandler(schemeIdentifier, func(handle *libvarnam.Varnam) (data interface{}, err error) {
// 		return handle.GetSuggestionsFilePath(), nil
// 	})

// 	db, err := sql.Open("sqlite3", filepath.(string))
// 	if err != nil {
// 		return nil, err
// 	}

// 	defer func() { _ = db.Close() }()

// 	// Making an index for all learned words so that download is faster
// 	// this needs to be removed when there is more clarity on how learned words needs to be handled
// 	if _, err = db.Exec("create index if not exists varnamd_download_only_learned on patterns_content (learned) where learned = 1;"); err != nil {
// 		return nil, err
// 	}

// 	q := `select id, word, confidence from words where id in (select distinct(word_id) from patterns_content where learned = 1) order by id asc limit ? offset ?;`

// 	rows, err := db.Query(q, downloadPageSize, downloadStart)
// 	if err != nil {
// 		return nil, err
// 	}

// 	if rows.Err() != nil {
// 		return nil, rows.Err()
// 	}

// 	defer func() { _ = rows.Close() }()

// 	var words []*word

// 	for rows.Next() {
// 		var (
// 			id, confidence int
// 			_word          string
// 		)

// 		_ = rows.Scan(&id, &_word, &confidence)

// 		words = append(words, &word{ID: id, Confidence: confidence, Word: _word})
// 	}

// 	return words, nil
// }

func reveseTransliterate(schemeIdentifier string, word string) (interface{}, error) {
	return getOrCreateHandler(schemeIdentifier, func(handle *govarnamgo.VarnamHandle) (data interface{}, err error) {
		return handle.ReverseTransliterate(word)
	})
}

func sendHandlerToChannel(schemeIdentifier string, handle *govarnamgo.VarnamHandle, ch chan *govarnamgo.VarnamHandle) {
	mutex.Lock()
	count := channelsCount[schemeIdentifier]
	mutex.Unlock()

	if count == getMaxHandleCount(schemeIdentifier) {
		log.Printf("Throw away handle")
		handle.Close()

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

func getSchemeFilePath(schemeIdentifier string) (interface{}, error) {
	return getOrCreateHandler(schemeIdentifier, func(handle *govarnamgo.VarnamHandle) (data interface{}, err error) {
		return handle.GetVSTPath(), nil
	})
}

func deleteWord(schemeIdentifier string, word string) (interface{}, error) {
	return getOrCreateHandler(schemeIdentifier, func(handle *govarnamgo.VarnamHandle) (data interface{}, err error) {
		return nil, handle.Unlearn(word)
	})
}

func searchSymbolTable(ctx context.Context, schemeIdentifier string, searchCondition govarnamgo.Symbol) (interface{}, error) {
	return getOrCreateHandler(schemeIdentifier, func(handle *govarnamgo.VarnamHandle) (data interface{}, err error) {
		return handle.SearchSymbolTable(ctx, searchCondition), err
	})
}
