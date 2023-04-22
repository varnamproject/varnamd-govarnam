package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/varnamproject/govarnam/govarnamgo"
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

type suggestionResponse struct {
	Word      string `json:"word"`
	Weight    int    `json:"weight"`
	LearnedOn int    `json:"learned_on"`
}

type advancedTransliterationResponse struct {
	standardResponse
	Input                        string               `json:"input"`
	ExactWords                   []suggestionResponse `json:"exact_words"`
	ExactMatches                 []suggestionResponse `json:"exact_matches"`
	DictionarySuggestions        []suggestionResponse `json:"dictionary_suggestions"`
	PatternDictionarySuggestions []suggestionResponse `json:"pattern_dictionary_suggestions"`
	TokenizerSuggestions         []suggestionResponse `json:"tokenizer_suggestions"`
	GreedyTokenized              []suggestionResponse `json:"greedy_tokenized"`
}

type metaResponse struct {
	// Result *libvarnam.CorpusDetails `json:"result"`
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
	Page       string `json:"page"`
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

	// Resolving a bug in echo
	// https://github.com/labstack/echo/issues/561
	var err error
	word, err = url.QueryUnescape(word)
	if err != nil {
		app.log.Printf("error in transliterating, err: %s", err.Error())
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error transliterating given string. message: %s", err.Error()))
	}

	cacheKey := fmt.Sprintf("tl-%s-%s", langCode, word)

	words, err := app.cache.GetString(cacheKey)
	if err != nil {
		result, err := transliterate(c.Request().Context(), langCode, word)
		if err != nil {
			app.log.Printf("error in transliterating, err: %s", err.Error())
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error transliterating given string. message: %s", err.Error()))
		}

		for _, sug := range result.([]govarnamgo.Suggestion) {
			words = append(words, sug.Word)
		}

		_ = app.cache.SetString(cacheKey, words...)
	}

	return c.JSON(http.StatusOK, transliterationResponse{standardResponse: newStandardResponse(), Result: words, Input: word})
}

func handleAdvancedTransliteration(c echo.Context) error {
	var (
		langCode = c.Param("langCode")
		word     = c.Param("word")
		app      = c.Get("app").(*App)
	)

	// Resolving a bug in echo
	// https://github.com/labstack/echo/issues/561
	var err error
	word, err = url.QueryUnescape(word)
	if err != nil {
		app.log.Printf("error in transliterating, err: %s", err.Error())
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error transliterating given string. message: %s", err.Error()))
	}

	var response advancedTransliterationResponse
	var cacheKey = fmt.Sprintf("atl-%s-%s", langCode, word)

	cached, err := app.cache.Get(cacheKey)
	if err == nil {
		response = cached.(advancedTransliterationResponse)
	} else {
		result, err := transliterateAdvanced(c.Request().Context(), langCode, word)
		if err != nil {
			app.log.Printf("error in transliterating, err: %s", err.Error())
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error transliterating given string. message: %s", err.Error()))
		}

		var varnamResult = result.(govarnamgo.TransliterationResult)

		for _, sug := range varnamResult.ExactWords {
			response.ExactWords = append(response.ExactWords, suggestionResponse(sug))
		}
		for _, sug := range varnamResult.ExactMatches {
			response.ExactMatches = append(response.ExactMatches, suggestionResponse(sug))
		}
		for _, sug := range varnamResult.DictionarySuggestions {
			response.DictionarySuggestions = append(response.DictionarySuggestions, suggestionResponse(sug))
		}
		for _, sug := range varnamResult.PatternDictionarySuggestions {
			response.PatternDictionarySuggestions = append(response.PatternDictionarySuggestions, suggestionResponse(sug))
		}
		for _, sug := range varnamResult.TokenizerSuggestions {
			response.TokenizerSuggestions = append(response.TokenizerSuggestions, suggestionResponse(sug))
		}
		for _, sug := range varnamResult.GreedyTokenized {
			response.GreedyTokenized = append(response.GreedyTokenized, suggestionResponse(sug))
		}

		_ = app.cache.Set(cacheKey, response)
	}

	response.Input = word

	// Don't return null for array responses
	if response.ExactWords == nil {
		response.ExactWords = []suggestionResponse{}
	}
	if response.ExactMatches == nil {
		response.ExactMatches = []suggestionResponse{}
	}
	if response.DictionarySuggestions == nil {
		response.DictionarySuggestions = []suggestionResponse{}
	}
	if response.PatternDictionarySuggestions == nil {
		response.PatternDictionarySuggestions = []suggestionResponse{}
	}
	if response.TokenizerSuggestions == nil {
		response.TokenizerSuggestions = []suggestionResponse{}
	}
	if response.GreedyTokenized == nil {
		response.GreedyTokenized = []suggestionResponse{}
	}

	response.standardResponse = newStandardResponse()

	return c.JSON(http.StatusOK, response)
}

func handleReverseTransliteration(c echo.Context) error {
	var (
		langCode = c.Param("langCode")
		word     = c.Param("word")
		app      = c.Get("app").(*App)
	)

	// Resolving a bug in echo
	// https://github.com/labstack/echo/issues/561
	var err error
	word, err = url.QueryUnescape(word)
	if err != nil {
		app.log.Printf("error in reverse transliterationg, err: %s", err.Error())
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error transliterating given string. message: %s", err.Error()))
	}

	// Separate namespace for reverse transliteration
	cacheKey := fmt.Sprintf("rtl-%s-%s", langCode, word)

	words, err := app.cache.GetString(cacheKey)
	if err != nil {
		result, err := reveseTransliterate(langCode, word)
		if err != nil {
			app.log.Printf("error in reverse transliterationg, err: %s", err.Error())
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error transliterating given string. message: %s", err.Error()))
		}

		for _, sug := range result.([]govarnamgo.Suggestion) {
			words = append(words, sug.Word)
		}

		_ = app.cache.SetString(cacheKey, words...)
	}

	if len(words) <= 0 {
		app.log.Printf("no reverse transliteration found for lang: %s word: %s", langCode, word)
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("no transliteration found for lanugage: %s, word: %s", langCode, word))
	}

	return c.JSON(http.StatusOK, transliterationResponse{standardResponse: newStandardResponse(), Result: words, Input: word})
}

// func handleMetadata(c echo.Context) error {
// 	var (
// 		schemeIdentifier = c.Param("langCode")
// 		app              = c.Get("app").(*App)
// 	)

// 	data, err := getOrCreateHandler(schemeIdentifier, func(handle *libvarnam.Varnam) (data interface{}, err error) {
// 		details, err := handle.GetCorpusDetails()
// 		if err != nil {
// 			return nil, err
// 		}

// 		return &metaResponse{Result: details, standardResponse: newStandardResponse()}, nil
// 	})
// 	if err != nil {
// 		app.log.Printf("error in getting corpus details for: %s, err: %s", schemeIdentifier, err.Error())
// 		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
// 	}

// 	return c.JSON(http.StatusOK, data)
// }

// func handleDownload(c echo.Context) error {
// 	var (
// 		langCode = c.Param("langCode")
// 		start, _ = strconv.Atoi(c.Param("downloadStart"))

// 		app = c.Get("app").(*App)
// 	)

// 	if start < 0 {
// 		return echo.NewHTTPError(http.StatusBadRequest, "invalid parameter")
// 	}

// 	fillCache := func(ctx context.Context, key string, dest groupcache.Sink) error {
// 		// cache miss, fetch from DB
// 		// key is in the form <schemeIdentifier>+<downloadStart>
// 		parts := strings.Split(key, "+")
// 		schemeID := parts[0]
// 		downloadStart, _ := strconv.Atoi(parts[1])

// 		words, err := getWords(schemeID, downloadStart)
// 		if err != nil {
// 			return err
// 		}

// 		response := downloadResponse{Count: len(words), Words: words, standardResponse: newStandardResponse()}

// 		b, err := json.Marshal(response)
// 		if err != nil {
// 			return err
// 		}

// 		// gzipping the response so that it can be served directly
// 		var gb bytes.Buffer
// 		gWriter := gzip.NewWriter(&gb)

// 		defer func() { _ = gWriter.Close() }()

// 		_, _ = gWriter.Write(b)
// 		_ = gWriter.Flush()

// 		if len(words) < downloadPageSize {
// 			varnamCtx, _ := ctx.(*varnamCacheContext)
// 			varnamCtx.Data = gb.Bytes()

// 			return errCacheSkipped
// 		}

// 		_ = dest.SetBytes(gb.Bytes())

// 		return nil
// 	}

// 	once.Do(func() {
// 		// Making the groups for groupcache
// 		// There will be one group for each language
// 		for _, scheme := range schemeDetails {
// 			group := groupcache.GetGroup(scheme.Identifier)
// 			if group == nil {
// 				// 100MB max size for cache
// 				group = groupcache.NewGroup(scheme.Identifier, 100<<20, groupcache.GetterFunc(fillCache))
// 			}
// 			cacheGroups[scheme.Identifier] = group
// 		}
// 	})

// 	cacheGroup := cacheGroups[langCode]
// 	ctx := varnamCacheContext{}

// 	var data []byte
// 	if err := cacheGroup.Get(&ctx, fmt.Sprintf("%s+%d", langCode, start), groupcache.AllocatingByteSliceSink(&data)); err != nil {
// 		if err == errCacheSkipped {
// 			c.Response().Header().Set("Content-Encoding", "gzip")
// 			return c.Blob(http.StatusOK, "application/json; charset=utf-8", ctx.Data)
// 		}

// 		app.log.Printf("error in fetching deta from cache: %s, err: %s", langCode, err.Error())

// 		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error getting metadata. message: %s", err.Error()))
// 	}

// 	c.Response().Header().Set("Content-Encoding", "gzip")

// 	return c.Blob(http.StatusOK, "application/json; charset=utf-8", data)
// }

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

func handleSchemeInfo(c echo.Context) error {
	var (
		schemeID = c.Param("schemeID")
	)

	sd, err := getSchemeDetails(schemeID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, sd)
}

func handleSchemeDefinitions(c echo.Context) error {
	var (
		schemeID = c.Param("schemeID")
		// app      = c.Get("app").(*App)
	)

	// do caching

	sd, err := getSchemeDetails(schemeID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	result, err := getSchemeDefinitions(c.Request().Context(), sd)

	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, schemeDefinition{standardResponse: newStandardResponse(), Details: sd, Definitions: result})
}

func handleSchemeLetterDefinitions(c echo.Context) error {
	var (
		schemeID = c.Param("schemeID")
		letter   = c.Param("letter")
		// app      = c.Get("app").(*App)
	)

	// do caching

	sd, err := getSchemeDetails(schemeID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	result, err := getSchemeLetterDefinitions(c.Request().Context(), sd, letter)

	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, schemeDefinition{standardResponse: newStandardResponse(), Details: sd, Definitions: result})
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

	cacheKey := fmt.Sprintf("tl-%s-%s", langCode, targs.Pattern)
	_, _ = app.cache.Delete(cacheKey)

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

func handlePackPageInfo(c echo.Context) error {
	var (
		langCode           = c.Param("langCode")
		packIdentifier     = c.Param("packIdentifier")
		packPageIdentifier = c.Param("packPageIdentifier")
	)

	pack, err := getPackPageInfo(langCode, packIdentifier, packPageIdentifier)
	if err != nil {
		statusCode := http.StatusBadRequest
		if err.Error() == "Pack page not found" {
			statusCode = http.StatusNotFound
		}

		return echo.NewHTTPError(statusCode, err.Error())
	}

	return c.JSON(http.StatusOK, pack)
}

func handlePacksDownload(c echo.Context) error {
	var (
		langCode           = c.Param("langCode")
		packIdentifier     = c.Param("packIdentifier")
		packPageIdentifier = c.Param("packPageIdentifier")
	)

	if _, err := getPackPageInfo(langCode, packIdentifier, packPageIdentifier); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	packFilePath, err := getPackFilePath(langCode, packIdentifier, packPageIdentifier)

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

	return c.Attachment(packFileGzipPath, packPageIdentifier)
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

	downloadResult, err = downloadPackFile(args.LangCode, args.Identifier, args.Page)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("error downloading pack: %s", err.Error()))
	}

	// Learn from pack file and don't remove it
	err = importLearningsFromFile(c, args.LangCode, downloadResult.FilePath, false)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Error importing from '%s'\n", err.Error()))
	}

	// Add pack.json with the installed pack pages
	err = updatePacksInfo(args.LangCode, downloadResult.Pack, downloadResult.Page)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, "success")
}
