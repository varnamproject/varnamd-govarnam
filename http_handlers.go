package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/rpc"
	"strconv"
	"strings"
	"time"

	"github.com/golang/groupcache"
	"github.com/gorilla/mux"
	"github.com/varnamproject/libvarnam-golang"
)

type varnamResponse struct {
	Result []string `json:"result"`
	Input  string   `json:"input"`
}

type downloadResponse struct {
	Count int     `json:"count"`
	Words []*word `json:"words"`
}

type requestParams struct {
	langCode      string
	word          string
	downloadStart int
}

func parseParams(r *http.Request) *requestParams {
	params := mux.Vars(r)
	downloadStart, _ := strconv.Atoi(params["downloadStart"])
	return &requestParams{langCode: params["langCode"], word: params["word"],
		downloadStart: downloadStart}
}

func corsHandler(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
		} else {
			h.ServeHTTP(w, r)
		}
	}
}

func recoverHandler(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic: %+v", err)
				http.Error(w, http.StatusText(500), 500)
			}
		}()
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func renderError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errorData := struct {
			Error string `json:"error"`
		}{
			err.Error(),
		}
		json.NewEncoder(w).Encode(errorData)
	}
}

func renderGzippedJSON(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Encoding", "gzip")
	w.Write(data)
}

func renderJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(data)
}

func getLanguageAndWord(r *http.Request) (langCode string, word string) {
	params := mux.Vars(r)
	langCode = params["langCode"]
	word = params["word"]
	return
}

func getLangCode(r *http.Request) string {
	params := mux.Vars(r)
	return params["langCode"]
}

func getWord(r *http.Request) string {
	params := mux.Vars(r)
	return params["word"]
}

func transliterationHandler(w http.ResponseWriter, r *http.Request) {
	langCode, word := getLanguageAndWord(r)
	words, err := transliterate(langCode, word)
	if err != nil {
		renderError(w, err)
	} else {
		renderJSON(w, varnamResponse{Result: words.([]string), Input: word})
	}
}

func reverseTransliterationHandler(w http.ResponseWriter, r *http.Request) {
	langCode, word := getLanguageAndWord(r)
	result, err := reveseTransliterate(langCode, word)
	if err != nil {
		renderError(w, err)
	} else {
		renderJSON(w, map[string]string{"result": result.(string)})
	}
}

func metadataHandler(w http.ResponseWriter, r *http.Request) {
	schemeIdentifier, _ := getLanguageAndWord(r)
	getOrCreateHandler(schemeIdentifier, func(handle *libvarnam.Varnam) (data interface{}, err error) {
		details, err := handle.GetCorpusDetails()
		if err != nil {
			renderError(w, err)
			return
		}
		renderJSON(w, details)
		return
	})
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	params := parseParams(r)
	if params.downloadStart < 0 {
		renderError(w, errors.New("Invalid parameters"))
		return
	}

	fillCache := func(ctx groupcache.Context, key string, dest groupcache.Sink) error {
		// cache miss, fetch from DB
		// key is in the form <schemeIdentifier>+<downloadStart>
		parts := strings.Split(key, "+")
		schemeId := parts[0]
		downloadStart, _ := strconv.Atoi(parts[1])
		words, err := getWords(schemeId, downloadStart)
		if err != nil {
			return err
		}

		response := downloadResponse{Count: len(words), Words: words}
		b, err := json.Marshal(response)
		if err != nil {
			return err
		}

		// gzipping the response so that it can be served directly
		var gb bytes.Buffer
		gWriter := gzip.NewWriter(&gb)
		defer gWriter.Close()
		gWriter.Write(b)
		gWriter.Flush()

		dest.SetBytes(gb.Bytes())
		return nil
	}

	once.Do(func() {
		// Making the groups for groupcache
		// There will be one group for each language
		for _, scheme := range schemeDetails {
			group := groupcache.GetGroup(scheme.Identifier)
			if group == nil {
				// 100MB max size for cache
				group = groupcache.NewGroup(scheme.Identifier, 100<<20, groupcache.GetterFunc(fillCache))
			}
			cacheGroups[scheme.Identifier] = group
		}
	})

	cacheGroup := cacheGroups[params.langCode]
	var data []byte
	err := cacheGroup.Get(nil,
		fmt.Sprintf("%s+%d", params.langCode, params.downloadStart), groupcache.AllocatingByteSliceSink(&data))
	if err != nil {
		renderError(w, err)
	}

	renderGzippedJSON(w, data)
}

func languagesHandler(w http.ResponseWriter, r *http.Request) {
	renderJSON(w, schemeDetails)
}

func repeatDial(times int) (client *rpc.Client, err error) {
	for times != 0 {
		client, err = rpc.DialHTTP("tcp", fmt.Sprintf("127.0.0.1:%d", learnPort))
		if err == nil {
			return
		}
		<-time.After(1 * time.Second)
		times--
	}
	return client, err
}

func learnHandler() http.HandlerFunc {
	client, err := repeatDial(20)
	if err != nil || client == nil {
		log.Fatalln("Unable to establish connection to learn only server:", err)
	}
	log.Printf("Connected to learn-only server at %d\n", learnPort)
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		var args Args
		if e := decoder.Decode(&args); e != nil {
			log.Println("Error in decoding ", e)
			renderError(w, e)
			return
		}
		var reply bool
		if err := client.Call("VarnamRPC.Learn", &args, &reply); err != nil {
			log.Println("Error in RPC ", err)
			renderError(w, err)
			return
		}
		renderJSON(w, "success")
	}
}
