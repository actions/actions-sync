package src

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	RepoNameRegExp   = regexp.MustCompile(`^[^/]+/\S+$`)
	ErrEmptyRepoList = errors.New("repo list cannot be empty")
)

type PullFlags struct {
	SourceURL, RepoName, RepoNameList, RepoNameListFile string
}

func (f *PullFlags) Init(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.SourceURL, "source-url", "https://github.com", "The domain to pull from")
	cmd.Flags().StringVar(&f.RepoName, "repo-name", "", "Single repository name to pull")
	cmd.Flags().StringVar(&f.RepoNameList, "repo-name-list", "", "Comma delimited list of repository names to pull")
	cmd.Flags().StringVar(&f.RepoNameListFile, "repo-name-list-file", "", "Path to file containing a list of repository names to pull")
}

func (f *PullFlags) Validate() Validations {
	var validations Validations
	if !f.HasAtLeastOneRepoFlag() {
		validations = append(validations, "one of -repo-name, -repo-name-list, -repo-name-list-file must be set")
	}
	return validations
}

func (f *PullFlags) HasAtLeastOneRepoFlag() bool {
	return f.RepoName != "" || f.RepoNameList != "" || f.RepoNameListFile != ""
}

func Pull(ctx context.Context, cacheDir string, flags *PullFlags) error {
	if flags.RepoNameList != "" {
		repoNames, err := getRepoNamesFromCSVString(flags.RepoNameList)
		if err != nil {
			return err
		}
		return PullManyWithGitImpl(ctx, flags.SourceURL, cacheDir, repoNames, gitImplementation{})
	}
	if flags.RepoNameListFile != "" {
		repoNames, err := getRepoNamesFromFile(flags.RepoNameListFile)
		if err != nil {
			return err
		}
		return PullManyWithGitImpl(ctx, flags.SourceURL, cacheDir, repoNames, gitImplementation{})
	}
	return PullWithGitImpl(ctx, flags.SourceURL, cacheDir, flags.RepoName, gitImplementation{})
}

func PullManyWithGitImpl(ctx context.Context, sourceURL, cacheDir string, repoNames []string, gitimpl GitImplementation) error {
	for _, repoName := range repoNames {
		if err := PullWithGitImpl(ctx, sourceURL, cacheDir, repoName, gitimpl); err != nil {
			return err
		}
	}
	return nil
}

func PullWithGitImpl(ctx context.Context, sourceURL, cacheDir string, repoName string, gitimpl GitImplementation) error {
	repoNameParts := strings.SplitN(repoName, ":", 2)
	originRepoName, err := validateRepoName(repoNameParts[0])
	if err != nil {
		return err
	}
	destRepoName := originRepoName
	if len(repoNameParts) > 1 {
		destRepoName, err = validateRepoName(repoNameParts[1])
		if err != nil {
			return err
		}
	}
	_, err = os.Stat(cacheDir)
	if err != nil {
		return err
	}

	dst := path.Join(cacheDir, destRepoName)

	if !gitimpl.RepositoryExists(dst) {
		fmt.Fprintf(os.Stdout, "pulling %s to %s ...\n", originRepoName, dst)
		_, err := gitimpl.CloneRepository(dst, &git.CloneOptions{
			SingleBranch: false,
			URL:          fmt.Sprintf("%s/%s", sourceURL, originRepoName),
		})
		if err != nil {
			if strings.Contains(err.Error(), "authentication required") {
				return fmt.Errorf("could not pull %s, the repository may require authentication or does not exist", originRepoName)
			}
			return err
		}
	}

	repo, err := gitimpl.NewGitRepository(dst)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "fetching * refs for %s ...\n", originRepoName)
	err = repo.FetchContext(ctx, &git.FetchOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/*:refs/heads/*"),
		},
		Tags: git.AllTags,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return err
	}

	return nil
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

func validateRepoName(name string) (string, error) {
	s := strings.TrimSpace(name)
	if RepoNameRegExp.MatchString(s) {
		return s, nil
	}
	return "", fmt.Errorf("`%s` is not a valid repo name", s)
}
