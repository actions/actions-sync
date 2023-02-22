package src

import (
	"fmt"
	"net/http"
	"os"
)

func PullPackagesForRepo(cacheDir,repoName, ghPatToken string) error {

	// Get the list of tags for the repo
	fmt.Fprintf(os.Stdout, "Pulling packages for repo: %s \n", repoName)
	
	ghPatTokenBase64Encoded := Base64Encode(ghPatToken)

	tags, err:= GetPackageTagsListFromGHCR(repoName, ghPatTokenBase64Encoded)
	if err != nil {
		return fmt.Errorf("Error getting list of tags for packages: %s", err)
	}

	//Pull package for each tag from GHCR
	for _, tag := range tags {
		err = PullPackageForTag(repoName, tag, ghPatTokenBase64Encoded, cacheDir)
		if err != nil {
			return fmt.Errorf("Error getting package: %s", err)
		}
	}
	return nil
}

func PullPackageForTag(repoName, tagName, ghPatTokenBase64Encoded, cacheDir string) error {

	// Get the layer digest package
	targzLayerDigest, err := GetLayerDigestFromGHCR(repoName, tagName, ghPatTokenBase64Encoded)
	if err != nil {
		return fmt.Errorf("Error getting layer digest for tag: %s", err)
	}

	// Get package from layer digest
	err = PullPackageFromLayerDigest(repoName, targzLayerDigest, tagName, ghPatTokenBase64Encoded, cacheDir)
	if err!= nil {
		return fmt.Errorf("Error pulling package from layer digest: %s", err)
	}
	
	return nil
}

func PullPackageFromLayerDigest(repoName, targzLayerDigestSHA, tagName, ghPatTokenBase64Encoded, cacheDir string) error {
	url := fmt.Sprintf("https://ghcr.io/v2/%s/blobs/%s", repoName, targzLayerDigestSHA)

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return err
    }

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ghPatTokenBase64Encoded))

    res, err := http.DefaultClient.Do(req)

    if err != nil {
        return err
    }
    defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected status code: %d", res.StatusCode)
	}

    file, err := os.Create(fmt.Sprintf("%s/%s-%s.tar.gz", cacheDir,repoName, tagName))
    if err != nil {
        return fmt.Errorf("Error creating file: %s", err)
    }

    defer file.Close()

    _, err = file.ReadFrom(res.Body)
    if err != nil {
        return fmt.Errorf("Error writing to file: %s", err)
    }

	return nil
}