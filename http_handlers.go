package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/golang/groupcache"
	"github.com/gorilla/mux"
	"github.com/varnamproject/libvarnam-golang"
)

var errCacheSkipped = errors.New("cache skipped")

// Context which gets passed into the groupcache fill function
// Data will be set if the cache returns CacheSkipped
type varnamCacheContext struct {
	Data []byte
	context.Context
}

type standardResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	At      string `json:"at"`
}

func newStandardResponse(err string) standardResponse {
	s := standardResponse{Success: true, Error: "", At: time.Now().UTC().String()}

	if err != "" {
		s.Error = err
		s.Success = false
	}

	return s
}

type transliterationResponse struct {
	standardResponse
	Result []string `json:"result"`
	Input  string   `json:"input"`
}

type metaResponse struct {
	Result *libvarnam.CorpusDetails `json:"result"`
	standardResponse
}

type downloadResponse struct {
	Count int     `json:"count"`
	Words []*word `json:"words"`
	standardResponse
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
				stack := make([]byte, 1024)
				stack = stack[:runtime.Stack(stack, false)]
				log.Printf("panic: %s\n%s", err, stack)
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

		errorData := newStandardResponse(err.Error())
		_ = json.NewEncoder(w).Encode(errorData)
	}
}

func renderGzippedJSON(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Encoding", "gzip")
	_, _ = w.Write(data)
}

func renderJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(data)
}

func getLanguageAndWord(r *http.Request) (langCode string, word string) {
	params := mux.Vars(r)
	langCode = params["langCode"]
	word = params["word"]

	return
}

// func getLangCode(r *http.Request) string {
// 	params := mux.Vars(r)
// 	return params["langCode"]
// }

// func getWord(r *http.Request) string {
// 	params := mux.Vars(r)
// 	return params["word"]
// }

func statusHandler(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(startedAt)

	resp := struct {
		Version string `json:"version"`
		Uptime  string `json:"uptime"`
		standardResponse
	}{
		varnamdVersion,
		uptime.String(),
		newStandardResponse(""),
	}

	renderJSON(w, resp)
}

func transliterationHandler(w http.ResponseWriter, r *http.Request) {
	langCode, word := getLanguageAndWord(r)
	words, err := transliterate(langCode, word)

	if err != nil {
		renderError(w, err)
	} else {
		renderJSON(w,
			transliterationResponse{standardResponse: newStandardResponse(""), Result: words.([]string), Input: word})
	}
}

func reverseTransliterationHandler(w http.ResponseWriter, r *http.Request) {
	langCode, word := getLanguageAndWord(r)
	result, err := reveseTransliterate(langCode, word)

	if err != nil {
		renderError(w, err)
	} else {
		response := struct {
			standardResponse
			Result string `json:"result"`
		}{
			newStandardResponse(""),
			result.(string),
		}
		renderJSON(w, response)
	}
}

func metadataHandler(w http.ResponseWriter, r *http.Request) {
	schemeIdentifier, _ := getLanguageAndWord(r)
	_, _ = getOrCreateHandler(schemeIdentifier, func(handle *libvarnam.Varnam) (data interface{}, err error) {
		details, err := handle.GetCorpusDetails()
		if err != nil {
			renderError(w, err)
			return
		}
		renderJSON(w, &metaResponse{Result: details, standardResponse: newStandardResponse("")})

		return
	})
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	params := parseParams(r)
	if params.downloadStart < 0 {
		renderError(w, errors.New("invalid parameters"))
		return
	}

	fillCache := func(ctx context.Context, key string, dest groupcache.Sink) error {
		// cache miss, fetch from DB
		// key is in the form <schemeIdentifier>+<downloadStart>
		parts := strings.Split(key, "+")
		schemeID := parts[0]
		downloadStart, _ := strconv.Atoi(parts[1])
		words, err := getWords(schemeID, downloadStart)

		if err != nil {
			return err
		}

		response := downloadResponse{Count: len(words), Words: words, standardResponse: newStandardResponse("")}
		b, err := json.Marshal(response)

		if err != nil {
			return err
		}

		// gzipping the response so that it can be served directly
		var gb bytes.Buffer
		gWriter := gzip.NewWriter(&gb)

		defer func() { _ = gWriter.Close() }()

		_, _ = gWriter.Write(b)
		_ = gWriter.Flush()

		if len(words) < downloadPageSize {
			varnamCtx, _ := ctx.(*varnamCacheContext)
			varnamCtx.Data = gb.Bytes()

			return errCacheSkipped
		}

		_ = dest.SetBytes(gb.Bytes())

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
	ctx := varnamCacheContext{}

	var data []byte

	if err := cacheGroup.Get(&ctx, fmt.Sprintf("%s+%d", params.langCode, params.downloadStart), groupcache.AllocatingByteSliceSink(&data)); err != nil {
		if err == errCacheSkipped {
			renderGzippedJSON(w, ctx.Data)
			return
		}

		renderError(w, err)

		return
	}

	renderGzippedJSON(w, data)
}

func languagesHandler(w http.ResponseWriter, r *http.Request) {
	renderJSON(w, schemeDetails)
}

func learnHandler(w http.ResponseWriter, r *http.Request) {
	var args Args

	decoder := json.NewDecoder(r.Body)

	if e := decoder.Decode(&args); e != nil {
		renderError(w, e)
		return
	}

	ch, ok := learnChannels[args.LangCode]

	if !ok {
		renderError(w, errors.New("unable to find language"))

		return
	}

	go func(word string) { ch <- word }(args.Word)

	renderJSON(w, "success")
}

func toggleDownloadEnabledStatus(w http.ResponseWriter, r *http.Request, status bool) {
	params := parseParams(r)
	err := varnamdConfig.setDownloadStatus(params.langCode, status)

	if err != nil {
		renderError(w, err)
	} else {
		renderJSON(w, newStandardResponse(""))
	}
}

func enableDownload(w http.ResponseWriter, r *http.Request) {
	toggleDownloadEnabledStatus(w, r, true)
}

func disableDownload(w http.ResponseWriter, r *http.Request) {
	toggleDownloadEnabledStatus(w, r, false)
}
