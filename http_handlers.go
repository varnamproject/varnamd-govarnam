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
	"github.com/labstack/echo/v4"
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

func newStandardResponse() standardResponse {
	return standardResponse{Success: true, At: time.Now().UTC().String()}
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

// Args to read.
type args struct {
	LangCode string `json:"lang"`
	Text     string `json:"text"`
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

// func getLangCode(r *http.Request) string {
// 	params := mux.Vars(r)
// 	return params["langCode"]
// }

// func getWord(r *http.Request) string {
// 	params := mux.Vars(r)
// 	return params["word"]
// }

func handleStatus(c echo.Context) error {
	uptime := time.Since(startedAt)

	resp := struct {
		Version string `json:"version"`
		Uptime  string `json:"uptime"`
		standardResponse
	}{
		varnamdVersion,
		uptime.String(),
		newStandardResponse(),
	}

	return c.JSON(http.StatusOK, resp)
}

func handleTransliteration(c echo.Context) error {
	var (
		langCode = c.Param("langCode")
		word     = c.Param("word")
	)

	// langCode, word := getLanguageAndWord(r)
	words, err := transliterate(langCode, word)

	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error transliterating given string. message: %s", err.Error()))
	}

	return c.JSON(http.StatusOK, transliterationResponse{standardResponse: newStandardResponse(), Result: words.([]string), Input: word})
}

func handleReverseTransliteration(c echo.Context) error {
	var (
		langCode = c.Param("langCode")
		word     = c.Param("word")
	)

	result, err := reveseTransliterate(langCode, word)

	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error transliterating given string. message: %s", err.Error()))
	}

	response := struct {
		standardResponse
		Result string `json:"result"`
	}{
		newStandardResponse(),
		result.(string),
	}

	return c.JSON(http.StatusOK, response)
}

func handleMetadata(c echo.Context) error {
	var schemeIdentifier = c.Param("langCode")

	data, err := getOrCreateHandler(schemeIdentifier, func(handle *libvarnam.Varnam) (data interface{}, err error) {
		details, err := handle.GetCorpusDetails()

		if err != nil {
			return nil, err
		}

		return &metaResponse{Result: details, standardResponse: newStandardResponse()}, nil
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	return c.JSON(http.StatusOK, data)
}

func handleDownload(c echo.Context) error {
	var (
		langCode = c.Param("langCode")
		// word     = c.Param("word")
	)

	start, _ := strconv.Atoi(c.Param("downloadStart"))

	if start < 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid parameter")
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

		response := downloadResponse{Count: len(words), Words: words, standardResponse: newStandardResponse()}
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

	cacheGroup := cacheGroups[langCode]
	ctx := varnamCacheContext{}

	var data []byte

	if err := cacheGroup.Get(&ctx, fmt.Sprintf("%s+%d", langCode, start), groupcache.AllocatingByteSliceSink(&data)); err != nil {
		if err == errCacheSkipped {
			c.Response().Header().Set("Content-Type", "application/json; charset=utf-8")
			c.Response().Header().Set("Content-Encoding", "gzip")

			return c.JSON(http.StatusOK, ctx.Data)
		}

		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	c.Response().Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Response().Header().Set("Content-Encoding", "gzip")

	return c.JSON(http.StatusOK, data)
}

func handleLanguages(c echo.Context) error {
	return c.JSON(http.StatusOK, schemeDetails)
}

func handlLearn(c echo.Context) error {
	var args args

	if err := c.Bind(&args); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	ch, ok := learnChannels[args.LangCode]

	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "unable to find language")
	}

	go func(word string) { ch <- word }(args.Text)

	return c.JSON(http.StatusOK, "success")
}

func toggleDownloadEnabledStatus(langCode string, status bool) (interface{}, error) {
	if err := varnamdConfig.setDownloadStatus(langCode, status); err != nil {
		return nil, err
	}

	return newStandardResponse(), nil
}

func handleEnableDownload(c echo.Context) error {
	var (
		langCode = c.Param("langCode")
	)

	data, err := toggleDownloadEnabledStatus(langCode, true)

	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	return c.JSON(http.StatusOK, data)
}

func handleDisableDownload(c echo.Context) error {
	var (
		langCode = c.Param("langCode")
	)

	data, err := toggleDownloadEnabledStatus(langCode, false)

	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	return c.JSON(http.StatusOK, data)
}
