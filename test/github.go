package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"net/http"
	"path"

	"github.com/google/go-github/v25/github"
	"github.com/gorilla/mux"
)

func main() {
	var port, gitDaemonURL string
	flag.StringVar(&port, "p", "", "")
	flag.StringVar(&gitDaemonURL, "git-daemon-url", "", "")
	flag.Parse()

	r := mux.NewRouter()
	r.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {})

	r.HandleFunc("/api/v3/orgs/{org}", func(w http.ResponseWriter, r *http.Request) {
		orgName := mux.Vars(r)["org"]

		org := github.Organization{Login: &orgName}
		b, _ := json.Marshal(org)
		_, err := w.Write(b)
		if err != nil {
			panic(err)
		}
	})

	r.HandleFunc("/api/v3/orgs/{org}/repos", func(w http.ResponseWriter, r *http.Request) {
		orgName := mux.Vars(r)["org"]
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		var repoReq struct {
			Name string `json:"name,omitempty"`
		}
		err = json.Unmarshal(b, &repoReq)
		if err != nil {
			panic(err)
		}

		cloneURL := gitDaemonURL + path.Join(orgName, repoReq.Name, ".git")
		repo := github.Repository{Name: &repoReq.Name, CloneURL: &cloneURL}
		b, _ = json.Marshal(repo)
		_, err = w.Write(b)
		if err != nil {
			panic(err)
		}
	}).Methods("POST")

	err := http.ListenAndServe(":"+port, r)
	if err != nil {
		panic(err)
	}
}
