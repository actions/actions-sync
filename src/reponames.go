package src

import (
	"fmt"
	"io/ioutil"
	"path"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

var (
	NwoRegExp        = regexp.MustCompile(`^[^/\s]+/[^/\s]+$`)
	ErrEmptyRepoList = errors.New("repo list cannot be empty")
	ErrEmptyCacheDir = errors.New("cache directory contains no actions to sync")
)

func getRepoNamesFromRepoFlags(flags *CommonFlags) ([]string, error) {
	if flags.RepoNameList != "" {
		return getRepoNamesFromCSVString(flags.RepoNameList)
	}

	if flags.RepoNameListFile != "" {
		return getRepoNamesFromFile(flags.RepoNameListFile)
	}

	if flags.RepoName != "" {
		return []string{flags.RepoName}, nil
	}

	return nil, nil
}

func getRepoNamesFromCacheDir(flags *CommonFlags) ([]string, error) {
	repoNames := make([]string, 0)

	orgDirs, err := ioutil.ReadDir(flags.CacheDir)
	if err != nil {
		return nil, errors.Wrapf(err, "error opening cache directory `%s`", flags.CacheDir)
	}
	for _, orgDir := range orgDirs {
		orgDirPath := path.Join(flags.CacheDir, orgDir.Name())
		if !orgDir.IsDir() {
			return nil, errors.Errorf("unexpected file in root of cache directory `%s`", orgDirPath)
		}
		repoDirs, err := ioutil.ReadDir(orgDirPath)
		if err != nil {
			return nil, errors.Wrapf(err, "error opening repository cache directory `%s`", orgDirPath)
		}
		for _, repoDir := range repoDirs {
			nwo := fmt.Sprintf("%s/%s", orgDir.Name(), repoDir.Name())
			repoNames = append(repoNames, nwo)
		}
	}

	if len(repoNames) == 0 {
		return nil, ErrEmptyCacheDir
	}

	return repoNames, nil
}

func getRepoNamesFromCSVString(csv string) ([]string, error) {
	repos := filterEmptyEntries(strings.Split(csv, ","))
	if len(repos) == 0 {
		return nil, ErrEmptyRepoList
	}
	return repos, nil
}

func getRepoNamesFromFile(file string) ([]string, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	repos := filterEmptyEntries(strings.Split(string(data), "\n"))
	if len(repos) == 0 {
		return nil, ErrEmptyRepoList
	}
	return repos, nil
}

func filterEmptyEntries(names []string) []string {
	filtered := []string{}
	for _, name := range names {
		if name != "" {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

func extractSourceDest(repoName string) (string, string, error) {
	repoNameParts := strings.Split(repoName, ":")
	if len(repoNameParts) > 2 {
		return "", "", fmt.Errorf("`%s` is not a valid repo name. Use a single colon to separate source and destination arguments. Example: `upstream_owner/upstream_repo:destination_owner/destination_repo`", repoName)
	}

	originNwo, err := validateNwo(repoNameParts[0])
	if err != nil {
		return "", "", err
	}

	destNwo := originNwo
	if len(repoNameParts) > 1 {
		destNwo, err = validateNwo(repoNameParts[1])
		if err != nil {
			return "", "", err
		}
	}

	return originNwo, destNwo, nil
}

func validateNwo(nwo string) (string, error) {
	s := strings.TrimSpace(nwo)
	if NwoRegExp.MatchString(s) {
		return s, nil
	}
	return "", fmt.Errorf("`%s` is not a valid repo name", s)
}

func splitNwo(nwo string) (string, string, error) {
	nwoParts := strings.Split(nwo, "/")
	if len(nwoParts) != 2 {
		return "", "", fmt.Errorf("`%s` is not a valid repo name", nwo)
	}

	return nwoParts[0], nwoParts[1], nil
}
