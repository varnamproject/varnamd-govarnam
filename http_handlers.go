package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/rpc"
	"time"

	"github.com/gorilla/mux"
)

type varnamResponse struct {
	Result []string `json:"result"`
	Input  string   `json:"input"`
}

func corsHandler(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
		} else {
			h.ServeHTTP(w, r)
		}
	}
}

func renderJson(w http.ResponseWriter, data interface{}, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintln(err)))
		return
	}
	marshal(data, w)
}

func marshal(item interface{}, w http.ResponseWriter) {
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

func languagesHandler(w http.ResponseWriter, r *http.Request) {
	renderJson(w, schemeDetails, nil)
}

func repeatDial(times int) (client *rpc.Client, err error) {
	for times != 0 {
		client, err = rpc.DialHTTP("tcp", fmt.Sprintf("127.0.0.1:%d", learnPort))
		if err == nil {
			return
		}
		<-time.After(300 * time.Millisecond)
		times--
	}
	return client, err
}

func learnHandler() http.HandlerFunc {
	client, err := repeatDial(10)
	if err != nil || client == nil {
		log.Fatalln("Unable to establish connection to learn only server:", err)
	}
	log.Printf("Connected to learn-only server at %d\n", learnPort)
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		var args Args
		if e := decoder.Decode(&args); e != nil {
			log.Println("Error in decoding ", e)
			renderJson(w, "", e)
			return
		}
		var reply bool
		if err := client.Call("VarnamRPC.Learn", &args, &reply); err != nil {
			log.Println("Error in RPC ", err)
			renderJson(w, "", err)
			return
		}
		renderJson(w, "success", nil)
	}
}
