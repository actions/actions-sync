package src

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
)

func PushPackagesForRepo(cacheDir, sourceToken, sourceRepoName, destinationURL, destinationToken, destinationRepoName, ghAPIUrl string) error {

	tags, err := ReadValidPackageTagsFromCache(cacheDir, sourceRepoName)
	if err != nil {
		return fmt.Errorf("Error getting list of tags for packages for repo %s: %s", sourceRepoName, err)
	}

	noOfTags := len(tags)
	var wg sync.WaitGroup
	wg.Add(noOfTags)

	for i := 0; i < noOfTags; i++ {
		go func(i int) {
			defer wg.Done()
			tag := tags[i]
			err = PushPackageForTag(cacheDir, sourceRepoName, sourceToken, destinationURL, destinationToken, destinationRepoName, tag, ghAPIUrl)
			if err != nil {
				fmt.Printf("Error pushing package for tag %s: %s", tag, err)
			}
		}(i)
	}
	wg.Wait()

	return nil
}

func PushPackageForTag(cacheDir, sourceRepoName, sourceToken, destinationURL, destinationToken, destinationRepoName, tagName, ghAPIUrl string) error {

	//Get release for tag from github
	release, err := GetReleaseForRepoTag(sourceRepoName, tagName, sourceToken, ghAPIUrl)
	if err != nil {
		return fmt.Errorf("Error getting release for tag %s: %s", tagName, err)
	}

	//Create release on destination
	releaseID, err := CreateReleaseForRepoTag(destinationURL, destinationToken, destinationRepoName, release)
	if err != nil {
		return fmt.Errorf("Error creating release for  %s:%s: %s", destinationRepoName, tagName, err)
	}

	//Push the package to destination
	err = PushPackageToDestination(cacheDir, destinationURL, destinationToken, destinationRepoName, tagName, sourceRepoName, releaseID)
	if err != nil {
		return fmt.Errorf("Error pushing package for tag %s to GHES: %s", tagName, err)
	}
	return nil
}

func PushPackageToDestination(cacheDir, destinationURL, token, destinationRepoName, tagName, sourceRepoName string, releaseID int) error {

	//get package for tag from cacheDir
	filePath := fmt.Sprintf("%s/%s-%s.tar.gz", cacheDir, sourceRepoName, tagName)

	packageFile, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer packageFile.Close()

	url := fmt.Sprintf("%s/api/v3/repos/%s/actions/package", destinationURL, destinationRepoName)

	fileData, err := ioutil.ReadAll(packageFile)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(fileData))
	if err != nil {
		return err
	}
	query := req.URL.Query()
	query.Add("release_id", fmt.Sprintf("%d", releaseID))
	req.URL.RawQuery = query.Encode()

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("Error publishing package on GHES: %s", err)
	}

	return nil
}
