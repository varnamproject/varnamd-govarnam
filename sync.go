package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/varnamproject/libvarnam-golang"
)

type syncDispatcher struct {
	quit   chan struct{}
	ticker *time.Ticker
}

func newSyncDispatcher(intervalInSeconds time.Duration) *syncDispatcher {
	return &syncDispatcher{ticker: time.NewTicker(intervalInSeconds), quit: make(chan struct{})}
}

func (s *syncDispatcher) start() {
	err := createSyncMetadataDir()
	if err != nil {
		fmt.Printf("Failed to create sync metadata directory. Sync will be disabled.\nActual error: %s\n", err.Error())
		return
	}
	for s := range varnamdConfig.SchemesToSync {
		// download cache directory for each of the languages
		err = createLearnQueueDir(s)
		if err != nil {
			fmt.Printf("Failed to create learn queue directory for '%s'. Sync will be disabled.\nActual error: %s\n", err.Error())
			return
		}
	}

	go func() {
		for {
			select {
			case <-s.ticker.C:
				syncWordsFromUpstream()
			case <-s.quit:
				s.ticker.Stop()
				return
			}
		}
	}()
}

func (s *syncDispatcher) stop() {
	close(s.quit)
}

func syncWordsFromUpstream() {
	log.Printf("Starting to sync words from %s\n", varnamdConfig.Upstream)
	for langCode := range varnamdConfig.SchemesToSync {
		log.Printf("Sync: %s\n", langCode)
		syncWordsFromUpstreamFor(langCode)
	}
	log.Printf("Finished sync words from %s\n", varnamdConfig.Upstream)
}

func syncWordsFromUpstreamFor(langCode string) {
	corpusDetails, err := getCorpusDetails(langCode)
	if err != nil {
		log.Printf("Error getting corpus details for '%s'. %s\n", langCode, err.Error())
		return
	}

	log.Printf("Corpus size: %d\n", corpusDetails.WordsCount)
	filesToLearn := make(chan string, 10)
	done := make(chan bool)
	go func() {
		offset := getDownloadOffset(langCode)
		log.Printf("Offset: %d\n", offset)
		for offset < corpusDetails.WordsCount {
			count, filePath, err := downloadWords(langCode, offset)
			if err != nil {
				log.Printf("Error downloading words for '%s'. %s\n", langCode, err.Error())
				break
			}

			err = setDownloadOffset(langCode, offset+count)
			if err != nil {
				log.Printf("Error setting download offset for '%s'. %s\n", langCode, err.Error())
				break
			}

			filesToLearn <- filePath
			offset = getDownloadOffset(langCode)
		}
		log.Println("Local copy is upto date. No need to download from upstream")
		close(filesToLearn)
	}()

	// Learn the downloaded files as and when it arrives on the channel
	go func() {
		for fileToLearn := range filesToLearn {
			log.Printf("Learning from %s\n", fileToLearn)
			getOrCreateHandler(langCode, func(handle *libvarnam.Varnam) (data interface{}, err error) {
				learnStatus, err := handle.LearnFromFile(fileToLearn)
				if err != nil {
					log.Printf("Error learning from '%s'\n", err.Error())
				} else {
					log.Printf("Learned from '%s'. TotalWords: %d, Failed: %d\n", fileToLearn, learnStatus.TotalWords, learnStatus.Failed)
				}
				return
			})

			err = os.Remove(fileToLearn)
			if err != nil {
				log.Printf("Error deleting '%s'. %s\n", fileToLearn, err.Error())
			}
		}
		done <- true
	}()

	<-done
}

func getCorpusDetails(langCode string) (*libvarnam.CorpusDetails, error) {
	url := fmt.Sprintf("%s/meta/%s", varnamdConfig.Upstream, langCode)
	log.Printf("Fetching corpus details for '%s'\n", langCode)
	var corpusDetails libvarnam.CorpusDetails
	err := getJSONResponse(url, &corpusDetails)
	if err != nil {
		return nil, err
	}
	return &corpusDetails, nil
}

// Downloads words from upstream starting from the specified offset and stores it locally in the learn queue
// Returns the number of words downloaded, local file path and error if any
func downloadWords(langCode string, offset int) (totalWordsDownloaded int, downloadedFilePath string, err error) {
	url := fmt.Sprintf("%s/download/%s/%d", varnamdConfig.Upstream, langCode, offset)
	var response downloadResponse
	err = getJSONResponse(url, &response)
	if err != nil {
		return 0, "", err
	}
	downloadedFilePath, err = transformAndPersistWords(langCode, &response)
	if err != nil {
		log.Printf("Download was successful, but failed to persist to local learn queue. %s\n", err.Error())
		return 0, "", err
	}

	return response.Count, downloadedFilePath, nil
}

func transformAndPersistWords(langCode string, dresp *downloadResponse) (string, error) {
	learnQueueDir := getLearnQueueDir(langCode)
	targetFile, err := ioutil.TempFile(learnQueueDir, langCode)
	if err != nil {
		return "", err
	}
	defer targetFile.Close()

	for _, word := range dresp.Words {
		_, err = targetFile.WriteString(fmt.Sprintf("%s %d\n", word.Word, word.Confidence))
		if err != nil {
			return "", err
		}
	}
	return targetFile.Name(), nil
}

func getJSONResponse(url string, output interface{}) error {
	log.Printf("GET: '%s'\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	jsonDecoder := json.NewDecoder(resp.Body)
	err = jsonDecoder.Decode(output)
	if err != nil {
		return err
	}
	return nil
}

func getDownloadOffset(langCode string) int {
	filePath := getDownloadOffsetMetadataFile(langCode)
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return 0
	}

	offset, err := strconv.Atoi(string(content))
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
	err := os.MkdirAll(queueDir, 0777)
	if err != nil {
		return err
	}
	return nil
}

func getLearnQueueDir(langCode string) string {
	syncDir := getSyncMetadataDir()
	queueDir := path.Join(syncDir, fmt.Sprintf("%s.learn.queue", langCode))
	return queueDir
}

func createSyncMetadataDir() error {
	syncDir := getSyncMetadataDir()
	err := os.MkdirAll(syncDir, 0777)
	if err != nil {
		return err
	}
	return nil
}

func getSyncMetadataDir() string {
	configDir := getConfigDir()
	syncDir := path.Join(configDir, "sync")
	return syncDir
}
