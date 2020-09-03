package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
		start, _ = strconv.Atoi(c.Param("downloadStart"))
	)

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
			c.Response().Header().Set("Content-Encoding", "gzip")
			return c.Blob(http.StatusOK, "application/json; charset=utf-8", ctx.Data)
		}

		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	c.Response().Header().Set("Content-Encoding", "gzip")
	return c.Blob(http.StatusOK, "application/json; charset=utf-8", data)
}

func handleLanguages(c echo.Context) error {
	return c.JSON(http.StatusOK, schemeDetails)
}

func handlLearn(c echo.Context) error {
	var a args

	c.Request().Header.Set("Content-Type", "application/json")

	if err := c.Bind(&a); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	ch, ok := learnChannels[a.LangCode]
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "unable to find language")
	}

	go func(word string) { ch <- word }(a.Text)

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
