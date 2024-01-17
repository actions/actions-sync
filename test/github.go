package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/google/go-github/v43/github"
	"github.com/gorilla/mux"
)

var authenticatedLogin string = "monalisa"

const existingOrg string = "org-already-exists"
const existingRepo string = "repo-already-exists"
const ghaeRepo string = "ghae-repo"
const xOAuthScopesHeader = "X-OAuth-Scopes"

//nolint:gocyclo
func main() {
	var port, gitDaemonURL string
	flag.StringVar(&port, "p", "", "")
	flag.StringVar(&gitDaemonURL, "git-daemon-url", "", "")
	flag.Parse()

	r := mux.NewRouter()
	r.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {})

	r.HandleFunc("/api/v3", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-github-enterprise-version", "GitHub AE")
		w.Header().Set(xOAuthScopesHeader, "site_admin")
	})

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
		b, err := io.ReadAll(r.Body)
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
		b, err := io.ReadAll(r.Body)
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
		b, err := io.ReadAll(r.Body)
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

	err := http.ListenAndServe(":"+port, r)
	if err != nil {
		panic(err)
	}
}
