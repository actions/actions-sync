package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"

	"github.com/google/go-github/v43/github"
	"github.com/gorilla/mux"
)

var authenticatedLogin string = "monalisa"
var existingOrg string = "org-already-exists"
var existingRepo string = "repo-already-exists"

func main() {
	var port, gitDaemonURL string
	flag.StringVar(&port, "p", "", "")
	flag.StringVar(&gitDaemonURL, "git-daemon-url", "", "")
	flag.Parse()

	r := mux.NewRouter()
	r.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {})

	r.HandleFunc("/api/v3/user", func(w http.ResponseWriter, r *http.Request) {
		currentUser := github.User{Login: &authenticatedLogin}
		b, _ := json.Marshal(currentUser)
		_, err := w.Write(b)
		if err != nil {
			panic(err)
		}
	})

	r.HandleFunc("/api/v3/admin/organizations", func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		var orgReq struct {
			Login string `json:"login,omitempty"`
			Admin string `json:"admin,omitempty"`
		}
		err = json.Unmarshal(b, &orgReq)
		if err != nil {
			panic(err)
		}

		if orgReq.Login == authenticatedLogin {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, err := w.Write([]byte(fmt.Sprintf("%s is a user, not an organization", orgReq.Login)))
			if err != nil {
				panic(err)
			}
		}

		if orgReq.Login == existingOrg {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, err := w.Write([]byte(fmt.Sprintf("Organization %s already exists", orgReq.Login)))
			if err != nil {
				panic(err)
			}
		}

		org := github.Organization{Login: &orgReq.Login}
		b, _ = json.Marshal(org)
		_, err = w.Write(b)
		if err != nil {
			panic(err)
		}
	}).Methods("POST")

	r.HandleFunc("/api/v3/orgs/{org}", func(w http.ResponseWriter, r *http.Request) {
		orgName := mux.Vars(r)["org"]

		if orgName != existingOrg {
			w.WriteHeader(http.StatusNotFound)
			_, err := w.Write([]byte(fmt.Sprintf("Organization %s not found", orgName)))
			if err != nil {
				panic(err)
			}
		}

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

		if repoReq.Name == "repo-already-exists" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, err := w.Write([]byte(fmt.Sprintf("Repo %s already exists", repoReq.Name)))
			if err != nil {
				panic(err)
			}
		}

		cloneURL := gitDaemonURL + path.Join(orgName, repoReq.Name, ".git")
		repo := github.Repository{Name: &repoReq.Name, CloneURL: &cloneURL}
		b, _ = json.Marshal(repo)
		_, err = w.Write(b)
		if err != nil {
			panic(err)
		}
	}).Methods("POST")

	r.HandleFunc("/api/v3/user/repos", func(w http.ResponseWriter, r *http.Request) {
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

		if repoReq.Name == existingRepo {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, err := w.Write([]byte(fmt.Sprintf("Repo %s already exists", repoReq.Name)))
			if err != nil {
				panic(err)
			}
		}

		cloneURL := gitDaemonURL + path.Join(authenticatedLogin, repoReq.Name, ".git")
		repo := github.Repository{Name: &repoReq.Name, CloneURL: &cloneURL}
		b, _ = json.Marshal(repo)
		_, err = w.Write(b)
		if err != nil {
			panic(err)
		}
	}).Methods("POST")

	r.HandleFunc("/api/v3/repos/{owner}/{repo}", func(w http.ResponseWriter, r *http.Request) {
		ownerName := mux.Vars(r)["owner"]
		repoName := mux.Vars(r)["repo"]

		if repoName != existingRepo {
			w.WriteHeader(http.StatusNotFound)
			_, err := w.Write([]byte(fmt.Sprintf("Repo %s not found", repoName)))
			if err != nil {
				panic(err)
			}
		}

		cloneURL := gitDaemonURL + path.Join(ownerName, repoName, ".git")
		org := github.Repository{Name: &repoName, CloneURL: &cloneURL}
		b, _ := json.Marshal(org)
		_, err := w.Write(b)
		if err != nil {
			panic(err)
		}
	})

	err := http.ListenAndServe(":"+port, r)
	if err != nil {
		panic(err)
	}
}
