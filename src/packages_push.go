package src

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"bytes"
	"os"
)

func PushPackagesForRepo(cacheDir, ghPatToken, sourceRepoName, destinationURL, destinationToken, destinationRepoName string) error {

	ghPatTokenBase64Encoded := Base64Encode(ghPatToken)

	tags, err:= GetPackageTagsListFromGHCR(sourceRepoName, ghPatTokenBase64Encoded)
	if err != nil {
		return fmt.Errorf("Error getting list of tags for packages: %s", err)
	}

	for _, tag := range tags {
		err = PushPackageForTag(cacheDir, sourceRepoName, ghPatToken, destinationURL, destinationToken, destinationRepoName, tag)
		if err != nil {
			return err
		}
	}

	return nil
}

func PushPackageForTag(cacheDir, sourceRepoName, ghPATToken, destinationURL, destinationToken, destinationRepoName, tagName string) error {

	//Get release for tag from github
	release, err := GetReleaseForRepoTag(sourceRepoName, tagName, ghPATToken)
	if err != nil {
		return fmt.Errorf("Error getting release for tag %s: %s", tagName, err)
	}

	//Create release on destination
	releaseId, err:= CreateReleaseForRepoTag(destinationURL, destinationToken, destinationRepoName, release)
	if err != nil {
		return fmt.Errorf("Error creating release for tag %s: %s", tagName, err)
	}
	if releaseId == 0 {
		//release and package already published so will be skipped as package overwrite is not supported
		return nil
	}

	//Push the package to destination
	err = PushPackageToDestination(cacheDir, destinationURL, destinationToken, destinationRepoName, tagName, sourceRepoName, releaseId)
	if err != nil {
		return fmt.Errorf("Error pushing package for tag %s: %s", tagName, err)
	}
	return nil
}



func PushPackageToDestination(cacheDir, destinationURL, token, destinationRepoName, tagName , sourceRepoName string, releaseId int) error {

	//get package for tag from cacheDir
	filePath:= fmt.Sprintf("%s/%s-%s.tar.gz", cacheDir, sourceRepoName, tagName)

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
    query.Add("release_id", fmt.Sprintf("%d", releaseId))
	req.URL.RawQuery = query.Encode()

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

	// fmt.Println(resp.Status)
	// body, err := ioutil.ReadAll(resp.Body)
    // if err != nil {
    //     fmt.Println("Error:", err)
    //     return err
    // }

    // fmt.Println(string(body))
	if resp.StatusCode != http.StatusCreated {
		fmt.Printf("Error publishing package on GHES: %s", err)
	}

	return nil
}
