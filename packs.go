package main

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
)

// PackPage Details of a pack page
type PackPage struct {
	Identifier  string `json:"identifier"` // Pack identifier is unique across all language packs. Example: ml-basic-1
	Page        int    `json:"page"`
	Description string `json:"description"`
	Size        int    `json:"size"`
}

// Pack Details of a pack
type Pack struct {
	Identifier  string     `json:"identifier"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	LangCode    string     `json:"lang"`
	Pages       []PackPage `json:"pages"`
}

type packDownload struct {
	Pack     *Pack
	Page     *PackPage
	FilePath string
}

var packsInfoCached []Pack

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// After a new pack download from upstream, update packs.json with installed packs
func updatePacksInfo(langCode string, pack *Pack, packPage *PackPage) error {
	packs, err := getPacksInfo()
	if err != nil {
		return err
	}

	var (
		existingPack *Pack = nil
	)

	for _, packR := range packs {
		if packR.Identifier == pack.Identifier {
			existingPack = &packR
			break
		}
	}

	if existingPack == nil {
		// will have one element
		pack.Pages = []PackPage{*packPage}
	} else {
		// Append new pack page
		existingPack.Pages = append(existingPack.Pages, *packPage)

		pack = existingPack
	}

	// Save pack.json
	packInfoPath := path.Join(getPacksDir(), pack.LangCode, pack.Identifier, "pack.json")
	file, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(packInfoPath, file, 0644)
	if err != nil {
		return err
	}

	packsInfoCached = nil

	return nil
}

// Download pack from upstream
func downloadPackFile(langCode, packIdentifier, packPageIdentifier string) (packDownload, error) {
	var (
		pack     *Pack
		packPage *PackPage = nil
	)

	packInstalled, _ := getPackPageInfo(langCode, packIdentifier, packPageIdentifier)
	if packInstalled != nil {
		return packDownload{}, fmt.Errorf("Pack already installed")
	}

	packInfoURL := fmt.Sprintf("%s/packs/%s/%s", varnamdConfig.upstream, langCode, packIdentifier)

	resp, err := http.Get(packInfoURL)
	if err != nil {
		return packDownload{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respData, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return packDownload{}, err
		}

		return packDownload{}, fmt.Errorf(string(respData))
	}

	if err = json.NewDecoder(resp.Body).Decode(&pack); err != nil {
		err := fmt.Errorf("Parsing packs JSON failed, err: %s", err.Error())
		return packDownload{}, err
	}

	for _, page := range pack.Pages {
		if page.Identifier == packPageIdentifier {
			packPage = &page
			break
		}
	}

	if packPage == nil {
		return packDownload{}, fmt.Errorf("Pack page not found")
	}

	fileURL := fmt.Sprintf("%s/packs/%s/%s/%s/download", varnamdConfig.upstream, langCode, packIdentifier, packPageIdentifier)
	fileDir := path.Join(getPacksDir(), langCode, packIdentifier)
	filePath := path.Join(fileDir, packPageIdentifier)

	if !fileExists(fileDir) {
		os.MkdirAll(fileDir, 0755)
	}

	resp, err = http.Get(fileURL)
	if err != nil {
		return packDownload{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respData, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return packDownload{}, err
		}

		return packDownload{}, fmt.Errorf(string(respData))
	}

	fz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return packDownload{}, err
	}
	defer fz.Close()

	out, err := os.Create(filePath)
	if err != nil {
		return packDownload{}, err
	}
	defer out.Close()

	// Write the gzip decoded content to file
	_, err = io.Copy(out, fz)

	if err != nil {
		return packDownload{}, err
	}

	return packDownload{Pack: pack, Page: packPage, FilePath: filePath}, nil
}

func getPackFilePath(langCode, packIdentifier, packPageIdentifier string) (string, error) {
	if _, err := getPackPageInfo(langCode, packIdentifier, packPageIdentifier); err != nil {
		return "", err
	}

	// Example: .varnamd/ml/ml-basic/ml-basic-1.vlf
	packFilePath := path.Join(getPacksDir(), langCode, packIdentifier, packPageIdentifier) + ".vlf"

	if !fileExists(packFilePath) {
		return "", errors.New("Pack file not found")
	}

	return packFilePath, nil
}

func getPackPageInfo(langCode string, packIdentifier string, packPageIdentifier string) (*PackPage, error) {
	pack, err := getPackInfo(langCode, packIdentifier)

	if err != nil {
		return nil, err
	}

	var packPage *PackPage = nil

	for _, page := range pack.Pages {
		if page.Identifier == packPageIdentifier {
			packPage = &page
			break
		}
	}

	if packPage == nil {
		return nil, fmt.Errorf("Pack page not found")
	}

	return packPage, nil
}

func getPackInfo(langCode string, packIdentifier string) (*Pack, error) {
	packs, err := getPacksLangInfo(langCode)

	if err != nil {
		return nil, err
	}

	for _, pack := range packs {
		if pack.Identifier == packIdentifier {
			return &pack, nil
		}
	}

	return nil, fmt.Errorf("Pack not found")
}

// Get packs by language
func getPacksLangInfo(langCode string) ([]Pack, error) {
	packs, err := getPacksInfo()

	if err != nil {
		return nil, err
	}

	var langPacks []Pack

	for _, pack := range packs {
		if pack.LangCode == langCode {
			langPacks = append(langPacks, pack)
		}
	}

	if len(langPacks) == 0 {
		return nil, fmt.Errorf("No packs found")
	}

	return langPacks, nil
}

func getPacksInfo() ([]Pack, error) {
	if err := createPacksDir(); err != nil {
		err := fmt.Errorf("Failed to create packs directory, err: %s", err.Error())
		return nil, err
	}

	if packsInfoCached != nil {
		return packsInfoCached, nil
	}

	var packsInfo []Pack

	files, err := ioutil.ReadDir(getPacksDir())
	if err != nil {
		return nil, err
	}

	for _, langFolder := range files {
		langFolderPath := path.Join(getPacksDir(), langFolder.Name())
		if langFolder.IsDir() {
			// inside ml
			langFolderFiles, err := ioutil.ReadDir(langFolderPath)

			if err != nil {
				return nil, err
			}

			for _, packFolder := range langFolderFiles {
				if packFolder.IsDir() {
					packInfoPath := path.Join(langFolderPath, packFolder.Name(), "pack.json")
					if fileExists(packInfoPath) {
						var packInfo Pack
						packsFile, _ := ioutil.ReadFile(packInfoPath)

						if err := json.Unmarshal(packsFile, &packInfo); err != nil {
							err := fmt.Errorf("Parsing packs JSON failed, err: %s", err.Error())
							return nil, err
						}

						packsInfo = append(packsInfo, packInfo)
					}
				}
			}
		}
	}

	packsInfoCached = packsInfo

	return packsInfo, nil
}

func createPacksDir() error {
	packsDir := getPacksDir()
	return os.MkdirAll(packsDir, 0750)
}

func getPacksDir() string {
	configDir := getConfigDir()
	return path.Join(configDir, "packs")
}
