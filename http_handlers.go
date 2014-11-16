package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

type varnamResponse struct {
	Result []string `json:"result"`
	Input  string   `json:"input"`
}

func renderJson(w http.ResponseWriter, data interface{}, err error) {
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintln(err)))
		return
	}
	marshal(data, w)
}

func marshal(item interface{}, w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(item)
}

func getLanguageAndWord(r *http.Request) (langCode string, word string) {
	params := mux.Vars(r)
	langCode = params["langCode"]
	word = params["word"]
	return
}

func transliterationHandler(w http.ResponseWriter, r *http.Request) {
	langCode, word := getLanguageAndWord(r)
	words, err := transliterate(langCode, word)
	renderJson(w, varnamResponse{Result: words, Input: word}, err)
}

func reverseTransliterationHandler(w http.ResponseWriter, r *http.Request) {
	langCode, word := getLanguageAndWord(r)
	result, err := reveseTransliterate(langCode, word)
	renderJson(w, map[string]string{"result": result}, err)
}

func learnHandler(w http.ResponseWriter, r *http.Request) {
	renderJson(w, "From learn", nil)
}
