package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/rpc"

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
	if err != nil {
		renderJson(w, nil, err)
	} else {
		renderJson(w, varnamResponse{Result: words.([]string), Input: word}, err)
	}
}

func reverseTransliterationHandler(w http.ResponseWriter, r *http.Request) {
	langCode, word := getLanguageAndWord(r)
	result, err := reveseTransliterate(langCode, word)
	if err != nil {
		renderJson(w, nil, err)
	} else {
		renderJson(w, map[string]string{"result": result.(string)}, err)
	}
}

func learnHandler() http.HandlerFunc {
	client, err := rpc.DialHTTP("tcp", "127.0.0.1:1234")
	if err != nil {
		log.Fatal("Unable to establish connection to learn only server:", err)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		langCode := r.FormValue("langCode")
		word := r.FormValue("word")
		args := &Args{langCode, word}
		var reply bool
		if err := client.Call("VarnamRPC.Learn", args, &reply); err != nil {
			log.Println("Error in RPC ", err)
			renderJson(w, "", err)
		}
		renderJson(w, "", nil)
	}
}
