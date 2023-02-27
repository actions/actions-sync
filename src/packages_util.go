package src

import (
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
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

func GetPackageTagsListFromGHCR(repoName, ghPatTokenBase64Encoded, ghcrHost string) ([]string, error) {

	//Get the list of tags for the repo packages from GHCR
	url := fmt.Sprintf("%s/v2/%s/tags/list", ghcrHost, repoName)
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

func GetLayerDigestFromGHCR(repoName, tagName, ghPatTokenBase64Encoded, ghcrHost string) (string, error) {
	
    url := fmt.Sprintf("%s/v2/%s/manifests/%s", ghcrHost, repoName, tagName)

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
	fmt.Println(url)
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

	//Release already exists
	if res.StatusCode == http.StatusUnprocessableEntity {
		return 0, nil
	}
	if res.StatusCode != http.StatusCreated {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return 0, fmt.Errorf("Failed to read response: %s", err)
		}

		return 0, fmt.Errorf("Error creating new release on GHES: %s:%s", res.Status, body)
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
func GetReleaseForRepoTag(repoName, tagName, ghPATToken, ghAPIUrl string) (Release, error) {

	url := fmt.Sprintf("%s/repos/%s/releases/tags/%s", ghAPIUrl, repoName, tagName)
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
        return release, fmt.Errorf("Unexpected status code: %d : %s", res.StatusCode, res.Status)
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

func WriteValidPackageTagsToCache(cacheDir, repoName string, tags []string) error {
	
	// Open a new file for writing
	file, err := os.Create(fmt.Sprintf("%s/%s/tags.txt", cacheDir, repoName))
    if err != nil {
        return fmt.Errorf("Error creating cache file: %s", err)
    }
    defer file.Close()

    // Marshal the array to JSON-encoded byte slice
    jsonData, err := json.Marshal(tags)
    if err != nil {
        return fmt.Errorf("Error marshalling tags to JSON: %s", err)
    }

    // Write the JSON-encoded data to the file
    _, err = file.Write(jsonData)
    if err != nil {
        return fmt.Errorf("Error writing tags to cache file: %s", err)
    }

	return nil
}

func ReadValidPackageTagsFromCache(cacheDir, repoName string) ([]string, error){

	file, err := os.Open(fmt.Sprintf("%s/%s/tags.txt", cacheDir, repoName))
    if err != nil {
        return nil, fmt.Errorf("Error opening cache file: %s", err)
    }
    defer file.Close()

	// Read the data from the file
	jsonData, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("Error reading cache file: %s", err)
	}

	// Unmarshal the JSON-encoded data into an array of strings
	var tags []string
	err = json.Unmarshal(jsonData, &tags)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshalling JSON: %s", err)
	}

	return tags, nil

}

func Base64Encode(token string) string {
	return b64.StdEncoding.EncodeToString([]byte(token))
}