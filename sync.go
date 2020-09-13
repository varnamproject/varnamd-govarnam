package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/varnamproject/varnamd/libvarnam"
)

type syncDispatcher struct {
	quit   chan struct{}
	force  chan bool // Send a TRUE message so that execution begins immediatly
	ticker *time.Ticker
}

func newSyncDispatcher(interval time.Duration) *syncDispatcher {
	return &syncDispatcher{ticker: time.NewTicker(interval), force: make(chan bool), quit: make(chan struct{})}
}

func (s *syncDispatcher) start() {
	if err := createSyncMetadataDir(); err != nil {
		fmt.Printf("Failed to create sync metadata directory. Sync will be disabled.\nActual error: %s\n", err.Error())
		return
	}

	for s := range varnamdConfig.schemesToDownload {
		// download cache directory for each of the languages
		if err := createLearnQueueDir(s); err != nil {
			fmt.Printf("Failed to create learn queue directory for '%s'. Sync will be disabled.\nActual error: %s\n", s, err.Error())
			return
		}
	}

	go func() {
		for {
			select {
			case <-s.ticker.C:
				performSync()
			case <-s.force:
				performSync()
			case <-s.quit:
				s.ticker.Stop()
				return
			}
		}
	}()
}

func (s *syncDispatcher) runNow() {
	s.force <- true
}

func performSync() {
	log.Println("---SYNC BEGIN---")
	log.Printf("Config: %v\n", varnamdConfig)

	syncWordsFromUpstream()

	log.Println("---SYNC DONE---")
}

func syncWordsFromUpstream() {
	for langCode := range varnamdConfig.schemesToDownload {
		log.Printf("Sync: %s\n", langCode)
		syncWordsFromUpstreamFor(langCode)
	}
}

func syncWordsFromUpstreamFor(langCode string) {
	corpusDetails, err := getCorpusDetails(langCode)
	if err != nil {
		log.Printf("Error getting corpus details for '%s'. %s\n", langCode, err.Error())
		return
	}

	localFilesToLearn := make(chan string, 100)
	downloadedFilesToLearn := make(chan string, 100)
	done := make(chan bool)

	// adding files which are remaining to learn in the local learn queue
	remainingFilesFromLastDownload := getFilesFromLearnQueue(langCode)
	go addFilesFromLocalLearnQueue(langCode, remainingFilesFromLastDownload, localFilesToLearn)
	go downloadAllWords(langCode, corpusDetails.WordsCount, downloadedFilesToLearn)
	go func() {
		learnAll(langCode, localFilesToLearn)
		learnAll(langCode, downloadedFilesToLearn)
		done <- true
	}()

	<-done
}

func addFilesFromLocalLearnQueue(langCode string, files []string, filesToLearn chan string) {
	if files != nil {
		log.Printf("Adding %d files to learn from local learn queue\n", len(files))

		for _, f := range files {
			filesToLearn <- f
		}
	} else {
		log.Printf("Local learn queue for '%s' is empty", langCode)
	}

	close(filesToLearn)
}

func downloadAllWords(langCode string, corpusSize int, output chan string) {
	for {
		offset := getDownloadOffset(langCode)
		log.Printf("Offset: %d\n", offset)

		if offset >= corpusSize {
			break
		}

		filePath, err := downloadWordsAndUpdateOffset(langCode, offset)
		if err != nil {
			break
		}

		output <- filePath
	}

	log.Println("Local copy is upto date. No need to download from upstream")

	close(output)
}

func learnAll(langCode string, filesToLearn chan string) {
	for fileToLearn := range filesToLearn {
		learnFromFile(langCode, fileToLearn)
	}
}

func learnFromFile(langCode, fileToLearn string) {
	start := time.Now()

	log.Printf("Learning from %s\n", fileToLearn)

	_, _ = getOrCreateHandler(langCode, func(handle *libvarnam.Varnam) (data interface{}, err error) {
		learnStatus, err := handle.LearnFromFile(fileToLearn)
		end := time.Now()
		if err != nil {
			log.Printf("Error learning from '%s'\n", err.Error())
		} else {
			log.Printf("Learned from '%s'. TotalWords: %d, Failed: %d. Took %s\n", fileToLearn, learnStatus.TotalWords, learnStatus.Failed, end.Sub(start))
		}

		if err = os.Remove(fileToLearn); err != nil {
			log.Printf("Error deleting '%s'. %s\n", fileToLearn, err.Error())
		}

		return
	})
}

func downloadWordsAndUpdateOffset(langCode string, offset int) (string, error) {
	count, filePath, err := downloadWords(langCode, offset)
	if err != nil {
		log.Printf("Error downloading words for '%s'. %s\n", langCode, err.Error())
		return "", err
	}

	if err = setDownloadOffset(langCode, offset+count); err != nil {
		log.Printf("Error setting download offset for '%s'. %s\n", langCode, err.Error())
		return "", err
	}

	return filePath, nil
}

func getCorpusDetails(langCode string) (*libvarnam.CorpusDetails, error) {
	var m metaResponse

	url := fmt.Sprintf("%s/meta/%s", varnamdConfig.upstream, langCode)
	log.Printf("Fetching corpus details for '%s'\n", langCode)

	if err := getJSONResponse(url, &m); err != nil {
		return nil, err
	}

	log.Printf("Corpus size: %d\n", m.Result.WordsCount)

	return m.Result, nil
}

// Downloads words from upstream starting from the specified offset and stores it locally in the learn queue
// Returns the number of words downloaded, local file path and error if any
func downloadWords(langCode string, offset int) (totalWordsDownloaded int, downloadedFilePath string, err error) {
	var response downloadResponse

	url := fmt.Sprintf("%s/download/%s/%d", varnamdConfig.upstream, langCode, offset)
	if err = getJSONResponse(url, &response); err != nil {
		return 0, "", err
	}

	downloadedFilePath, err = transformAndPersistWords(langCode, offset, &response)
	if err != nil {
		log.Printf("Download was successful, but failed to persist to local learn queue. %s\n", err.Error())
		return 0, "", err
	}

	return response.Count, downloadedFilePath, nil
}

func transformAndPersistWords(langCode string, offset int, dresp *downloadResponse) (string, error) {
	learnQueueDir := getLearnQueueDir(langCode)

	targetFile, err := os.Create(path.Join(learnQueueDir, fmt.Sprintf("%s.%d", langCode, offset)))
	if err != nil {
		return "", err
	}

	defer func() { _ = targetFile.Close() }()

	for _, word := range dresp.Words {
		if _, err = targetFile.WriteString(fmt.Sprintf("%s %d\n", word.Word, word.Confidence)); err != nil {
			return "", err
		}
	}

	return targetFile.Name(), nil
}

func getFilesFromLearnQueue(langCode string) []string {
	var files []string

	learnQueueDir := getLearnQueueDir(langCode)

	queueContents, err := ioutil.ReadDir(learnQueueDir)
	if err != nil {
		return nil
	}

	for _, c := range queueContents {
		if !c.IsDir() {
			files = append(files, path.Join(learnQueueDir, c.Name()))
		}
	}

	return files
}

func getJSONResponse(url string, output interface{}) error {
	log.Printf("GET: '%s'\n", url)

	resp, err := http.Get(url) // #nosec G107
	if err != nil {
		return err
	}

	defer func() { _ = resp.Body.Close() }()

	jsonDecoder := json.NewDecoder(resp.Body)

	return jsonDecoder.Decode(output)
}

func getDownloadOffset(langCode string) int {
	filePath := getDownloadOffsetMetadataFile(langCode)

	content, err := ioutil.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return 0
	}

	offset, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		return 0
	}

	return offset
}

func setDownloadOffset(langCode string, offset int) error {
	filePath := getDownloadOffsetMetadataFile(langCode)
	return ioutil.WriteFile(filePath, []byte(fmt.Sprintf("%d", offset)), 0666)
}

func getDownloadOffsetMetadataFile(langCode string) string {
	syncDir := getSyncMetadataDir()
	return path.Join(syncDir, fmt.Sprintf("%s.download.offset", langCode))
}

func createLearnQueueDir(langCode string) error {
	queueDir := getLearnQueueDir(langCode)
	return os.MkdirAll(queueDir, 0750)
}

func getLearnQueueDir(langCode string) string {
	syncDir := getSyncMetadataDir()
	queueDir := path.Join(syncDir, fmt.Sprintf("%s.learn.queue", langCode))

	return queueDir
}

func createSyncMetadataDir() error {
	syncDir := getSyncMetadataDir()
	return os.MkdirAll(syncDir, 0750)
}

func getSyncMetadataDir() string {
	configDir := getConfigDir()
	return path.Join(configDir, "sync")
}
