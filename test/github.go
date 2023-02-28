package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/google/go-github/v43/github"
	"github.com/gorilla/mux"
)

var authenticatedLogin string = "monalisa"
var releaseCreationCounter int = 0

const existingOrg string = "org-already-exists"
const existingRepo string = "repo-already-exists"
const ghaeRepo string = "ghae-repo"
const xOAuthScopesHeader = "X-OAuth-Scopes"

const packagesMockDataPath = "test/fixtures/packages"
const tag1 = "1.0.0"
const tag2 = "1.0.1"
const tag3 = "1.0.2"
type Release struct {
	ID                   int    `json:"id"`
	TagName              string `json:"tag_name"`
	TargetCommitish      string `json:"target_commitish"`
	Name                 string `json:"name"`
	Body                 string `json:"body"`
	Draft                bool   `json:"draft"`
	Prerelease           bool   `json:"prerelease"`
	GenerateReleaseNotes bool   `json:"generate_release_notes"`
}

//nolint:gocyclo
func main() {
	var port, gitDaemonURL, ghcrURL, ghcrPort, ghAPIURL, ghAPIPort string
	flag.StringVar(&port, "p", "", "")
	flag.StringVar(&gitDaemonURL, "git-daemon-url", "", "")
	flag.StringVar(&ghcrPort, "ghcr-port", "", "")
	flag.StringVar(&ghcrURL, "ghcr-url", "", "")
	flag.StringVar(&ghAPIPort, "gh-api-port", "", "")
	flag.StringVar(&ghAPIURL, "gh-api-url", "", "")

	flag.Parse()

	r := mux.NewRouter()
	ghcrRouter := mux.NewRouter()
	ghAPIRouter := mux.NewRouter()

	r.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {})
	r.HandleFunc("/api/v3", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-github-enterprise-version", "GitHub AE")
		w.Header().Set(xOAuthScopesHeader, "site_admin")
	})

	r.HandleFunc("/api/v3/repos/{owner}/{repo}/actions/package", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}).Methods("POST")

	r.HandleFunc("/api/v3/repos/{owner}/{repo}/releases", func(w http.ResponseWriter, r *http.Request) {

		var release Release
		err := json.NewDecoder(r.Body).Decode(&release)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Error decoding request body: %v", err)
			return
		}

		releaseCreationCounter++

		createdRelease := Release{
			ID:              releaseCreationCounter,
			TagName:         release.TagName,
			TargetCommitish: release.TargetCommitish,
			Name:            release.Name,
			Body:            release.Body,
			Draft:           release.Draft,
			Prerelease:      release.Prerelease,
		}

		// Return the created release in the response.
		w.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(w).Encode(createdRelease)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error encoding response body: %v", err)
			return
		}

	}).Methods("POST")

	r.HandleFunc("/api/v3/user", func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if strings.Contains(token, "ghaetoken") {
			w.Header().Set("x-github-enterprise-version", "GitHub AE")
		}
		currentUser := github.User{Login: &authenticatedLogin}
		b, _ := json.Marshal(currentUser)
		_, err := w.Write(b)
		if err != nil {
			panic(err)
		}
	})

	r.HandleFunc("/api/v3/admin/users/ghes-admin/authorizations", func(w http.ResponseWriter, r *http.Request) {
		token := "token"
		auth := github.Authorization{Token: &token}
		b, _ := json.Marshal(auth)
		_, err := w.Write(b)
		if err != nil {
			panic(err)
		}
	}).Methods("POST")

	r.HandleFunc("/api/v3/admin/users/ghae-admin/authorizations", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-github-enterprise-version", "GitHub AE")
		token := "ghaetoken"
		auth := github.Authorization{Token: &token}
		b, _ := json.Marshal(auth)
		_, err := w.Write(b)
		if err != nil {
			panic(err)
		}
	}).Methods("POST")

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
			_, err := w.Write([]byte(fmt.Sprintf("%s is a user, not an organization", html.EscapeString(orgReq.Login))))
			if err != nil {
				panic(err)
			}
		}

		if orgReq.Login == existingOrg {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, err := w.Write([]byte(fmt.Sprintf("Organization %s already exists", html.EscapeString(orgReq.Login))))
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
			_, err := w.Write([]byte(fmt.Sprintf("Organization %s not found", html.EscapeString(orgName))))
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
			Name       string `json:"name,omitempty"`
			Visibility string `json:"visibility,omitempty"`
		}
		err = json.Unmarshal(b, &repoReq)
		if err != nil {
			panic(err)
		}

		var errString string = ""
		// check visibility requirements
		if repoReq.Name == ghaeRepo {
			if repoReq.Visibility != "internal" {
				errString = fmt.Sprintf("Provided repo visibility %s for GHAE must be internal", repoReq.Visibility)
			}
		} else {
			if repoReq.Visibility != "public" {
				errString = fmt.Sprintf("Provided repo visibility %s for GHES must be public", repoReq.Visibility)
			}
		}

		// check if we are testing existing Repo
		if repoReq.Name == existingRepo {
			errString = fmt.Sprintf("Repo %s already exists", html.EscapeString(repoReq.Name))
		}

		// if there is an error throw it back
		if errString != "" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, err := w.Write([]byte(errString))
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
			_, err := w.Write([]byte(fmt.Sprintf("Repo %s already exists", html.EscapeString(repoReq.Name))))
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
			_, err := w.Write([]byte(fmt.Sprintf("Repo %s not found", html.EscapeString(repoName))))
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

	ghcrRouter.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {})

	ghcrRouter.HandleFunc("/v2/{owner}/{repo}/tags/list", func(w http.ResponseWriter, r *http.Request) {

		tagsList, err := ioutil.ReadFile(fmt.Sprintf("%s/org/repo/%s.json", packagesMockDataPath, "tags-list"))
		if err != nil {
			log.Fatal(err)
		}

		var jsonData map[string]interface{}
		err = json.Unmarshal(tagsList, &jsonData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(jsonData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)

	})

	ghcrRouter.HandleFunc("/v2/{owner}/{repo}/manifests/{tag}", func(w http.ResponseWriter, r *http.Request) {

		tag := mux.Vars(r)["tag"]

		var manifestTag string

		switch tag {
		case tag1:
			manifestTag = tag1
		case tag2:
			manifestTag = tag2
		case tag3:
			manifestTag = tag3
		}
		
		manifest, err := ioutil.ReadFile(fmt.Sprintf("%s/org/repo/manifest-%s.json", packagesMockDataPath, manifestTag))
		if err != nil {
			log.Fatal(err)
		}

		var jsonData map[string]interface{}
		err = json.Unmarshal(manifest, &jsonData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(jsonData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)

	})

	ghcrRouter.HandleFunc("/v2/{owner}/{repo}/blobs/{sha}", func(w http.ResponseWriter, r *http.Request) {

		file, err := os.Open(fmt.Sprintf("%s/org/repo/layer.tar.gz", packagesMockDataPath))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer file.Close()

		gz, err := gzip.NewReader(file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer gz.Close()

		blob, err := ioutil.ReadAll(gz)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(blob)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	})

	ghAPIRouter.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {})

	ghAPIRouter.HandleFunc("/repos/{owner}/{repo}/releases/tags/{tag}", func(w http.ResponseWriter, r *http.Request) {

		tag := mux.Vars(r)["tag"]

		var manifestTag string

		switch tag {
		case tag1:
			manifestTag = tag1
		case tag2:
			manifestTag = tag2
		case tag3:
			manifestTag = tag3
		}

		manifest, err := ioutil.ReadFile(fmt.Sprintf("%s/org/repo/release-%s.json", packagesMockDataPath, manifestTag))
		if err != nil {
			log.Fatal(err)
		}

		var jsonData map[string]interface{}
		err = json.Unmarshal(manifest, &jsonData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(jsonData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)

	})

	destGitServer := &http.Server{
		Handler: r,
		Addr:    ":" + port,
	}

	ghcrServer := &http.Server{
		Handler: ghcrRouter,
		Addr:    ":" + ghcrPort,
	}

	ghAPIServer := &http.Server{
		Handler: ghAPIRouter,
		Addr:    ":" + ghAPIPort,
	}

	//go routines to start multiple servers parallelly
	go func() {
		err := destGitServer.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}()
	go func() {
		err := ghcrServer.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}()
	go func() {
		err := ghAPIServer.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}()

	// Waits for a signal to shutdown the servers.
	<-make(chan struct{})

}
