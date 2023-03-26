package src

import (
	"fmt"
	"net/http"
	"os"
)

func PullPackagesForRepo(cacheDir, repoName, sourceToken, ghcrHost string) error {

	fmt.Fprintf(os.Stdout, "Pulling packages for repo: %s \n", repoName)

	sourceTokenBase64Encoded := Base64Encode(sourceToken)

	// Get the list of tags for the repo
	tags, err := GetPackageTagsListFromGHCR(repoName, sourceTokenBase64Encoded, ghcrHost)
	if err != nil {
		return fmt.Errorf("Error getting list of tags for packages: %s", err)
	}

	var validTags []string
	//Pull package for each tag from GHCR
	for _, tag := range tags {
		err = PullPackageForTag(repoName, tag, sourceTokenBase64Encoded, cacheDir, ghcrHost)
		if err != nil {
			return fmt.Errorf("Error getting package: %s", err)
		}
		validTags = append(validTags, tag)
	}

	//Write valid package tags in file for pushing to destination
	err = WriteValidPackageTagsToCache(cacheDir, repoName, validTags)
	if err != nil {
		return fmt.Errorf("Error writing valid package tags to file: %s", err)
	}
	return nil
}

func PullPackageForTag(repoName, tagName, sourceTokenBase64Encoded, cacheDir, ghcrHost string) error {

	// Get the layer digest package
	targzLayerDigest, err := GetLayerDigestFromGHCR(repoName, tagName, sourceTokenBase64Encoded, ghcrHost)
	if err != nil {
		return fmt.Errorf("Error getting layer digest for tag: %s", err)
	}

	// Get package from layer digest
	err = PullPackageFromLayerDigest(repoName, targzLayerDigest, tagName, sourceTokenBase64Encoded, cacheDir, ghcrHost)
	if err != nil {
		return fmt.Errorf("Error pulling package from layer digest: %s", err)
	}

	return nil
}

func PullPackageFromLayerDigest(repoName, targzLayerDigestSHA, tagName, sourceTokenBase64Encoded, cacheDir, ghcrHost string) error {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", ghcrHost, repoName, targzLayerDigestSHA)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sourceTokenBase64Encoded))

	res, err := http.DefaultClient.Do(req)

	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected status code: %d", res.StatusCode)
	}

	file, err := os.Create(fmt.Sprintf("%s/%s-%s.tar.gz", cacheDir, repoName, tagName))
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
