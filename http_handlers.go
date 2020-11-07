package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang/groupcache"
	"github.com/labstack/echo/v4"
	"github.com/varnamproject/varnamd/libvarnam"
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

//TrainArgs read the incoming data
type trainArgs struct {
	Pattern string `json:"pattern"`
	Word    string `json:"word"`
}

//TrainBulkArgs read the incoming data for bulk training.
type trainBulkArgs struct {
	Pattern []string `json:"pattern"`
	Word    string   `json:"word"`
}

// packDownloadArgs is the args to request a pack download from upstream
type packDownloadArgs struct {
	LangCode   string `json:"lang"`
	Identifier string `json:"pack"`
	Version    string `json:"version"`
}

func handleStatus(c echo.Context) error {
	uptime := time.Since(startedAt)

	resp := struct {
		Version string `json:"version"`
		Uptime  string `json:"uptime"`
		standardResponse
	}{
		buildVersion + "-" + buildDate,
		uptime.String(),
		newStandardResponse(),
	}

	return c.JSON(http.StatusOK, resp)
}

func handleTransliteration(c echo.Context) error {
	var (
		langCode = c.Param("langCode")
		word     = c.Param("word")
		app      = c.Get("app").(*App)
	)

	words, err := app.cache.Get(langCode, word)
	if err != nil {
		w, err := transliterate(langCode, word)
		if err != nil {
			app.log.Printf("error in transliterationg, err: %s", err.Error())
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error transliterating given string. message: %s", err.Error()))
		}

		words, _ = w.([]string)
		_ = app.cache.Set(langCode, word, words...)
	}

	return c.JSON(http.StatusOK, transliterationResponse{standardResponse: newStandardResponse(), Result: words, Input: word})
}

func handleReverseTransliteration(c echo.Context) error {
	var (
		langCode = c.Param("langCode")
		word     = c.Param("word")
		app      = c.Get("app").(*App)
	)

	result, err := app.cache.Get(langCode, word)
	if err != nil {
		res, err := reveseTransliterate(langCode, word)
		if err != nil {
			app.log.Printf("error in reverse transliterationg, err: %s", err.Error())
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error transliterating given string. message: %s", err.Error()))
		}

		result = []string{res.(string)}
		_ = app.cache.Set(langCode, word, res.(string))
	}

	if len(result) <= 0 {
		app.log.Printf("no reverse transliteration found for lang: %s word: %s", langCode, word)
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("no transliteration found for lanugage: %s, word: %s", langCode, word))
	}

	response := struct {
		standardResponse
		Result string `json:"result"`
	}{
		newStandardResponse(),
		result[0],
	}

	return c.JSON(http.StatusOK, response)
}

func handleMetadata(c echo.Context) error {
	var (
		schemeIdentifier = c.Param("langCode")
		app              = c.Get("app").(*App)
	)

	data, err := getOrCreateHandler(schemeIdentifier, func(handle *libvarnam.Varnam) (data interface{}, err error) {
		details, err := handle.GetCorpusDetails()
		if err != nil {
			return nil, err
		}

		return &metaResponse{Result: details, standardResponse: newStandardResponse()}, nil
	})
	if err != nil {
		app.log.Printf("error in getting corpus details for: %s, err: %s", schemeIdentifier, err.Error())
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	return c.JSON(http.StatusOK, data)
}

func handleDownload(c echo.Context) error {
	var (
		langCode = c.Param("langCode")
		start, _ = strconv.Atoi(c.Param("downloadStart"))

		app = c.Get("app").(*App)
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

		app.log.Printf("error in fetching deta from cache: %s, err: %s", langCode, err.Error())

		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	c.Response().Header().Set("Content-Encoding", "gzip")

	return c.Blob(http.StatusOK, "application/json; charset=utf-8", data)
}

func handleLanguages(c echo.Context) error {
	return c.JSON(http.StatusOK, schemeDetails)
}

func handleLanguageDownload(c echo.Context) error {
	var (
		langCode = c.Param("langCode")
	)

	filepath, err := getSchemeFilePath(langCode)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error: %s", err.Error()))
	}

	return c.Attachment(filepath.(string), langCode+".vst")
}

func handleLearn(c echo.Context) error {
	var (
		a args

		app = c.Get("app").(*App)
	)

	if err := c.Bind(&a); err != nil {
		app.log.Printf("error in binding request details for learn, err: %s", err.Error())
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	ch, ok := learnChannels[a.LangCode]
	if !ok {
		app.log.Printf("unknown language requested to learn: %s", a.LangCode)
		return echo.NewHTTPError(http.StatusBadRequest, "unable to find language")
	}

	go func(word string) { ch <- word }(a.Text)

	return c.JSON(http.StatusOK, "success")
}

func handleLearnFileUpload(c echo.Context) error {
	var (
		app      = c.Get("app").(*App)
		langCode = c.Param("langCode")
	)

	// Multipart form
	form, err := c.MultipartForm()
	if err != nil {
		app.log.Printf("failed to read form from request, language: %s, error: %s", langCode, err.Error())
		return echo.NewHTTPError(http.StatusBadRequest, "request data not found")
	}

	files, ok := form.File["files"]
	if !ok {
		app.log.Printf("files not found, language: %s", langCode)
		return echo.NewHTTPError(http.StatusBadRequest, "no files were uploaded")
	}

	if _, ok := learnChannels[langCode]; !ok {
		app.log.Printf("learn file upload error: unknown language requested to learn: %s", langCode)
		return echo.NewHTTPError(http.StatusBadRequest, "unable to find language to train")
	}

	// Copy files first
	for _, file := range files {
		// Source
		src, err := file.Open()
		if err != nil {
			app.log.Printf("learn file upload error, err: %s", err.Error())
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		// Destination
		tempDir, err := ioutil.TempDir(os.TempDir(), "varnamd")
		if err != nil {
			app.log.Printf("learn file upload error, err: %s", err.Error())
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		dst, err := os.Create(filepath.Join(tempDir, file.Filename))
		if err != nil {
			app.log.Printf("learn file upload error, err: %s", err.Error())
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		// Copy
		if _, err = io.Copy(dst, src); err != nil {
			app.log.Printf("learn file upload error, err: %s", err.Error())
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		// Explicitely closing resources.
		_ = dst.Close()
		_ = src.Close()

		learnWordsFromFile(c, langCode, dst.Name(), true)
	}

	return c.JSON(http.StatusOK, "success")
}

func handleTrain(c echo.Context) error {
	var (
		targs    trainArgs
		app      = c.Get("app").(*App)
		langCode = c.Param("langCode")
	)

	c.Request().Header.Set("Content-Type", "application/json")

	if err := c.Bind(&targs); err != nil {
		app.log.Printf("error reading request, err: %s", err.Error())
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	ch, ok := trainChannel[langCode]
	if !ok {
		app.log.Printf("unknown language requested to learn: %s", langCode)
		return echo.NewHTTPError(http.StatusBadRequest, "unable to find language to train")
	}

	go func(args trainArgs) { ch <- args }(targs)

	_, _ = app.cache.Delete(langCode, targs.Pattern)

	return c.JSON(200, "Word Trained")
}

// handleTrainBulk is an endpoint for training words in the following format.
// {[
// 	{word, patterns: []},
// 	{word, patterns: []},
// 	{word, patterns: []},
// 	{word, patterns: []}
// ]}
// It will covert each bulk arg to trainArg and will send to train channel.
// Training is happened at listenForWords method.
func handleTrainBulk(c echo.Context) error {
	var (
		bulkArgs []trainBulkArgs
		app      = c.Get("app").(*App)
		langCode = c.Param("langCode")
	)

	if err := c.Bind(&bulkArgs); err != nil {
		app.log.Printf("error reading request, err: %s", err.Error())
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	ch, ok := trainChannel[langCode]
	if !ok {
		app.log.Printf("unknown language requested to learn: %s", langCode)
		return echo.NewHTTPError(http.StatusBadRequest, "unable to find language to train")
	}

	for _, v := range bulkArgs {
		for _, p := range v.Pattern {
			go func(args trainArgs) {
				ch <- args
			}(trainArgs{
				Pattern: p,
				Word:    v.Word,
			})
		}
	}

	return c.JSON(200, "Words Trained")
}

// Delete a word
func handleDelete(c echo.Context) error {
	var (
		a args

		app = c.Get("app").(*App)
	)

	c.Request().Header.Set("Content-Type", "application/json")

	if err := c.Bind(&a); err != nil {
		app.log.Printf("error in binding request details for delete, err: %s", err.Error())
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	if _, err := deleteWord(a.LangCode, a.Text); err != nil {
		app.log.Printf("error deleting word, err: %s", err.Error())
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error: %s", err.Error()))
	}

	app.cache.Clear()

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

		app = c.Get("app").(*App)
	)

	data, err := toggleDownloadEnabledStatus(langCode, true)
	if err != nil {
		app.log.Printf("failed to toggle download enable, err: %s", err.Error())
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	return c.JSON(http.StatusOK, data)
}

func handleDisableDownload(c echo.Context) error {
	var (
		langCode = c.Param("langCode")
		app      = c.Get("app").(*App)
	)

	data, err := toggleDownloadEnabledStatus(langCode, false)
	if err != nil {
		app.log.Printf("failed to disable download, err: %s", err.Error())
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	return c.JSON(http.StatusOK, data)
}

// handleIndex is the root handler that renders the Javascript frontend.
func handleIndex(c echo.Context) error {
	app, _ := c.Get("app").(*App)

	b, err := app.fs.Read("/index.html")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	c.Response().Header().Set("Content-Type", "text/html")

	return c.String(http.StatusOK, string(b))
}

func handlePacks(c echo.Context) error {
	var (
		langCode = c.Param("langCode")
		app      = c.Get("app").(*App)
	)

	if langCode != "" {
		pack, err := getPacksLangInfo(langCode)
		if err != nil {
			statusCode := http.StatusBadRequest
			if err.Error() == "No packs found" {
				statusCode = http.StatusNotFound
			}

			app.log.Printf("error reading packs, err: %s", err.Error())
			return echo.NewHTTPError(statusCode, err.Error())
		}
		return c.JSON(http.StatusOK, pack)
	}

	packs, err := getPacksInfo()
	if err != nil {
		app.log.Printf("error reading packs, err: %s", err.Error())
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, packs)
}

func handlePackInfo(c echo.Context) error {
	var (
		langCode       = c.Param("langCode")
		packIdentifier = c.Param("packIdentifier")
	)

	pack, err := getPackInfo(langCode, packIdentifier)
	if err != nil {
		statusCode := http.StatusBadRequest
		if err.Error() == "Pack not found" {
			statusCode = http.StatusNotFound
		}

		return echo.NewHTTPError(statusCode, err.Error())
	}

	return c.JSON(http.StatusOK, pack)
}

func handlePackVersionInfo(c echo.Context) error {
	var (
		langCode              = c.Param("langCode")
		packIdentifier        = c.Param("packIdentifier")
		packVersionIdentifier = c.Param("packVersionIdentifier")
	)

	pack, err := getPackVersionInfo(langCode, packIdentifier, packVersionIdentifier)
	if err != nil {
		statusCode := http.StatusBadRequest
		if err.Error() == "Pack version not found" {
			statusCode = http.StatusNotFound
		}

		return echo.NewHTTPError(statusCode, err.Error())
	}

	return c.JSON(http.StatusOK, pack)
}

func handlePacksDownload(c echo.Context) error {
	var (
		langCode              = c.Param("langCode")
		packIdentifier        = c.Param("packIdentifier")
		packVersionIdentifier = c.Param("packVersionIdentifier")
	)

	if _, err := getPackVersionInfo(langCode, packIdentifier, packVersionIdentifier); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	packFilePath, err := getPackFilePath(langCode, packIdentifier, packVersionIdentifier)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	packFileGzipPath := path.Join(packFilePath + ".gzip")

	if !fileExists(packFileGzipPath) {
		// compress into gzip
		packFileBytes, err := ioutil.ReadFile(packFilePath)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		var gb bytes.Buffer
		w := gzip.NewWriter(&gb)
		w.Write(packFileBytes)
		w.Close()

		err = ioutil.WriteFile(packFileGzipPath, gb.Bytes(), 0644)

		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	return c.Attachment(packFileGzipPath, packVersionIdentifier)
}

// varnamd Admin can download packs from upstream
// This is an internal function
func handlePackDownloadRequest(c echo.Context) error {
	var (
		args           packDownloadArgs
		app            = c.Get("app").(*App)
		err            error
		downloadResult packDownload
	)

	if err := c.Bind(&args); err != nil {
		app.log.Printf("error reading request, err: %s", err.Error())
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
	}

	downloadResult, err = downloadPackFile(args.LangCode, args.Identifier, args.Version)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error downloading pack: %s", err.Error()))
	}

	// Learn from pack file and don't remove it
	err = importLearningsFromFile(c, args.LangCode, downloadResult.FilePath, false)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Error importing from '%s'\n", err.Error()))
	}

	// Add pack.json with the installed pack versions
	err = updatePacksInfo(args.LangCode, downloadResult.Pack, downloadResult.Version)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, "success")
}
