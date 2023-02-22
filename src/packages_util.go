package src

import (
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"bytes"
	"io/ioutil"
	"net/http"
)

type Tags struct {
	Tags []string `json:"tags"`
}

type Manifest struct {
	Layers []Layer `json:"layers"`
}

type Layer struct {
	MediaType string `json:"mediaType"`
	Digest string `json:"digest"`
}

type Release struct {
	Id int `json:"id"`
	TagName string `json:"tag_name"`
	TargetCommitish string `json:"target_commitish"`
	Name string `json:"name"`
	Body string `json:"body"`
	Draft bool `json:"draft"`
	Prerelease bool `json:"prerelease"`
	GenerateReleaseNotes bool `json:"generate_release_notes"`
}

func GetPackageTagsListFromGHCR(repoName, ghPatTokenBase64Encoded string) ([]string, error) {

	//Get the list of tags for the repo packages from GHCR
	url := fmt.Sprintf("https://ghcr.io/v2/%s/tags/list", repoName)
	fmt.Println("Getting list of tags for packages from GHCR. Url: ", url)
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("Error getting list of tags for packages: %s", err)
    }

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ghPatTokenBase64Encoded))

    res, err := http.DefaultClient.Do(req)
    if err != nil {
		return nil, fmt.Errorf("Error getting list of tags for packages: %s", err)
    }

    defer res.Body.Close()

    if res.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("Unexpected status code: %d", res.StatusCode)
    }

	var tags Tags
	if err := json.NewDecoder(res.Body).Decode(&tags); err != nil {
        return nil, err
    }

	return tags.Tags, nil
}

func GetLayerDigestFromGHCR(repoName, tagName, ghPatTokenBase64Encoded string) (string, error) {
	
    url := fmt.Sprintf("https://ghcr.io/v2/%s/manifests/%s", repoName, tagName)

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return "", err
    }

    req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ghPatTokenBase64Encoded))

    res, err := http.DefaultClient.Do(req)

    if err != nil {
        return "", err
    }
    defer res.Body.Close()
    if res.StatusCode != http.StatusOK {
        return "", fmt.Errorf("Unexpected status code: %d", res.StatusCode)
    }

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	var manifest Manifest
	err = json.Unmarshal(body, &manifest)
	if err != nil {
		return "", err
	}

	return manifest.Layers[0].Digest, nil
}

func CreateReleaseForRepoTag(destinationURL, token, repoName string, release Release) (int, error) {

	url := fmt.Sprintf("%s/api/v3/repos/%s/releases", destinationURL, repoName)

	newRelease := Release{
		TagName: release.TagName,
		TargetCommitish: release.TargetCommitish,
		Name: release.Name,
		Body: release.Body,
		Draft: release.Draft,
		Prerelease: release.Prerelease,
		GenerateReleaseNotes: true,
	}

	reqBody, _ := json.Marshal(newRelease)

    req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
    if err != nil {
		return 0, err
    }

    req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

    res, err := http.DefaultClient.Do(req)
    if err != nil {
        return 0, err
    }
    defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return 0, fmt.Errorf("Error creating new release on GHES: %s", res.Status)
	}

	body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return 0, fmt.Errorf("Failed to read response: %s", err)
			
		}

	var generatedRelease Release
	err = json.Unmarshal(body, &generatedRelease)
	if err != nil {
		return 0, err
	}

	 return generatedRelease.Id, nil
}
func GetReleaseForRepoTag(repoName, tagName, ghPATToken string) (Release, error) {

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repoName, tagName)
	var release Release

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return release, err
    }
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ghPATToken))
    res, err := http.DefaultClient.Do(req)

    if err != nil {
        return release, err
    }

    defer res.Body.Close()

    if res.StatusCode != http.StatusOK {
        return release, fmt.Errorf("Unexpected status code: %d", res.StatusCode)
    }

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return release, err
	}

	err = json.Unmarshal(body, &release)
	if err != nil {
		return release, err
	}

	return release, nil

}

func Base64Encode(token string) string {
	return b64.StdEncoding.EncodeToString([]byte(token))
}