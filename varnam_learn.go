package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"gitlab.com/subins2000/govarnam/govarnamgo"
)

const defaultChanSize = 1000

var (
	learnChannels map[string]chan string
	trainChannel  map[string]chan trainArgs
)

// initChannels method will initialize learn and train channels.
func (app *App) initChannels() {
	learnChannels = make(map[string]chan string)
	trainChannel = make(map[string]chan trainArgs)

	for _, scheme := range schemeDetails {
		learnChannels[scheme.Identifier] = make(chan string, defaultChanSize)
		trainChannel[scheme.Identifier] = make(chan trainArgs, defaultChanSize)

		handle, err := govarnamgo.InitFromID(scheme.Identifier)
		if err != nil {
			app.log.Fatal("Unable to initialize varnam for lang", scheme.LangCode)
		}

		go app.listenForWords(scheme.Identifier, handle)
	}
}

func (app *App) listenForWords(lang string, handle *govarnamgo.VarnamHandle) {
	for {
		select {
		case word := <-learnChannels[lang]:
			if err := handle.Learn(strings.TrimSpace(word), 0); err != nil {
				app.log.Printf("Failed to learn %s. %s\n", word, err.Error())
			}
		case args := <-trainChannel[lang]:
			if err := handle.Train(strings.TrimSpace(args.Pattern), strings.TrimSpace(args.Word)); err != nil {
				app.log.Printf("error training word: %s, pattern: %s, err:%s", args.Word, args.Pattern, err.Error())
			}
		}
	}
}

func learnWordsFromFile(c echo.Context, langCode string, fileToLearn string, removeFile bool) {
	c.Response().WriteHeader(http.StatusOK)

	start := time.Now()

	sendOutput := func(msg string) {
		_, _ = c.Response().Write([]byte(msg))
		c.Response().Flush()
	}

	sendOutput(fmt.Sprintf("Learning from %s\n", fileToLearn))

	_, _ = getOrCreateHandler(langCode, func(handle *govarnamgo.VarnamHandle) (data interface{}, err error) {
		learnStatus, verr := handle.LearnFromFile(fileToLearn)
		end := time.Now()

		if verr != nil {
			sendOutput(fmt.Sprintf("Error learning: '%s'\n", verr.Error()))
		} else {
			sendOutput(fmt.Sprintf("Finished Learning. TotalWords: %d, Failed: %d. Took %s\n", learnStatus.TotalWords, learnStatus.FailedWords, end.Sub(start)))
		}

		if removeFile {
			if err = os.Remove(fileToLearn); err != nil {
				sendOutput(fmt.Sprintf("Error deleting '%s'. %s\n", fileToLearn, err.Error()))
			}
		}

		return
	})
}

func importLearningsFromFile(c echo.Context, langCode string, fileToImport string, removeFile bool) error {
	c.Response().WriteHeader(http.StatusOK)

	start := time.Now()

	sendOutput := func(msg string) {
		_, _ = c.Response().Write([]byte(msg))
		c.Response().Flush()
	}

	sendOutput(fmt.Sprintf("Importing from %s\n", fileToImport))

	var importError error

	_, _ = getOrCreateHandler(langCode, func(handle *govarnamgo.VarnamHandle) (data interface{}, err error) {
		err = handle.Import(fileToImport)
		end := time.Now()

		if err != nil {
			importError = err
		} else {
			sendOutput(fmt.Sprintf("Import completed. Took %s\n", end.Sub(start)))
		}

		if removeFile {
			if err = os.Remove(fileToImport); err != nil {
				sendOutput(fmt.Sprintf("Error deleting '%s'. %s\n", fileToImport, err.Error()))
			}
		}
		return
	})

	return importError
}
