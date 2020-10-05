package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
)

// PackVersion Details of a pack version
type PackVersion struct {
	Identifier  string `json:"identifier"` // Pack identifier is unique across all language packs. Example: ml-basic-1
	Version     int    `json:"version"`
	Description string `json:"description"`
	Size        int    `json:"size"`
}

// Pack Details of a pack
type Pack struct {
	Identifier  string        `json:"identifier"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	LangCode    string        `json:"lang"`
	Versions    []PackVersion `json:"versions"`
}

type packDownload struct {
	Pack     *Pack
	Version  *PackVersion
	FilePath string
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// After a new pack download from upstream, update packs.json with installed packs
func updatePacksInfo(langCode string, pack *Pack, packVersion *PackVersion) error {
	packs, err := getPacksInfo()
	if err != nil {
		return err
	}

	var (
		existingPackIndex int
		existingPack      *Pack = nil
	)

	for index, packR := range packs {
		if packR.Identifier == pack.Identifier {
			existingPackIndex = index
			existingPack = &packR
			break
		}
	}

	if existingPack == nil {
		// will have one element
		pack.Versions = []PackVersion{*packVersion}

		// Append new pack
		packs = append(packs, *pack)
	} else {
		// Append new pack version
		existingPack.Versions = append(existingPack.Versions, *packVersion)

		packs[existingPackIndex] = *existingPack
	}

	file, err := json.MarshalIndent(packs, "", "  ")
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(getPacksInfoPath(), file, 0644)
	if err != nil {
		return err
	}

	return nil
}

// Download pack from upstream
func downloadPackFile(langCode, packIdentifier, packVersionIdentifier string) (packDownload, error) {
	var (
		pack        *Pack
		packVersion *PackVersion = nil
	)

	packInstalled, _ := getPackVersionInfo(langCode, packIdentifier, packVersionIdentifier)
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

	for _, version := range pack.Versions {
		if version.Identifier == packVersionIdentifier {
			packVersion = &version
			break
		}
	}

	if packVersion == nil {
		return packDownload{}, fmt.Errorf("Pack version not found")
	}

	fileURL := fmt.Sprintf("%s/packs/%s/%s/%s/download", varnamdConfig.upstream, langCode, packIdentifier, packVersionIdentifier)
	fileDir := path.Join(getPacksDir(), langCode)
	filePath := path.Join(fileDir, packVersionIdentifier)

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

	out, err := os.Create(filePath)
	if err != nil {
		return packDownload{}, err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)

	if err != nil {
		return packDownload{}, err
	}

	return packDownload{Pack: pack, Version: packVersion, FilePath: filePath}, nil
}

func getPackFilePath(langCode, packIdentifier, packVersionIdentifier string) (string, error) {
	if _, err := getPackVersionInfo(langCode, packIdentifier, packVersionIdentifier); err != nil {
		return "", err
	}

	// Example: .varnamd/ml/ml-basic-1
	packFilePath := path.Join(getPacksDir(), langCode, packVersionIdentifier)

	if !fileExists(packFilePath) {
		return "", errors.New("Pack file not found")
	}

	return packFilePath, nil
}

func getPackVersionInfo(langCode string, packIdentifier string, packVersionIdentifier string) (*PackVersion, error) {
	pack, err := getPackInfo(langCode, packIdentifier)

	if err != nil {
		return nil, err
	}

	var packVersion *PackVersion = nil

	for _, version := range pack.Versions {
		if version.Identifier == packVersionIdentifier {
			packVersion = &version
			break
		}
	}

	if packVersion == nil {
		return nil, fmt.Errorf("Pack version not found")
	}

	return packVersion, nil
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

	packsFilePath := getPacksInfoPath()

	if !fileExists(packsFilePath) {
		file, err := os.Create(packsFilePath)
		if err != nil {
			return nil, err
		}
		file.WriteString("[]")
		defer file.Close()
	}

	packsFile, _ := ioutil.ReadFile(packsFilePath)

	var packsInfo []Pack

	if err := json.Unmarshal(packsFile, &packsInfo); err != nil {
		err := fmt.Errorf("Parsing packs JSON failed, err: %s", err.Error())
		return nil, err
	}

	return packsInfo, nil
}

func getPacksInfoPath() string {
	return getPacksDir() + "/packs.json"
}

func createPacksDir() error {
	packsDir := getPacksDir()
	return os.MkdirAll(packsDir, 0644)
}

func getPacksDir() string {
	configDir := getConfigDir()
	return path.Join(configDir, "packs")
}
