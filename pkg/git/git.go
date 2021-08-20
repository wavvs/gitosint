package git

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"io/ioutil"
	"os"
	"os/signal"
	"sync"
	"time"

	memfs "github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/google/uuid"
)

var auth transport.AuthMethod

func ConvertCommitMetadata(metadata CommitMetadata) map[string][]string {
	meta := make(map[string][]string)
	for k := range metadata {
		if _, ok := meta[k.Email]; !ok {
			meta[k.Email] = []string{k.Name}
		} else {
			contains := func() bool {
				for _, name := range meta[k.Email] {
					if k.Name == name {
						return true
					}
				}
				return false
			}()

			if !contains {
				meta[k.Email] = append(meta[k.Email], k.Name)
			}
		}
	}
	return meta
}

func createInMemoryRepo(emails []string) (*git.Repository, error) {
	storer := memory.NewStorage()
	fs := memfs.New()
	repo, err := git.Init(storer, fs)
	if err != nil {
		return nil, err
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return nil, err
	}
	filePath := uuid.NewString()
	file, err := fs.Create(filePath)
	if err != nil {
		return nil, err
	}

	var opts git.CommitOptions
	for _, email := range emails {
		hash := md5.Sum([]byte(email))
		_, err := file.Write(hash[:])
		if err != nil {
			return nil, err
		}
		_, err = worktree.Add(filePath)
		if err != nil {
			return nil, err
		}
		opts = git.CommitOptions{
			Author: &object.Signature{
				Name:  email,
				Email: email,
				When:  time.Now(),
			},
			Committer: &object.Signature{
				Name:  email,
				Email: email,
				When:  time.Now(),
			},
		}
		_, err = worktree.Commit(hex.EncodeToString(hash[:]), &opts)
		if err != nil {
			return nil, err
		}
	}

	return repo, nil
}

func createRemote(repo *git.Repository, remoteURL string) (*git.Repository, error) {
	_, err := repo.CreateRemote(&config.RemoteConfig{
		Name: remoteName,
		URLs: []string{remoteURL},
	})

	if err != nil {
		return nil, err
	}

	return repo, nil
}

func pushInMemoryRepo(repo *git.Repository) error {
	return repo.Push(&git.PushOptions{
		RemoteName:      remoteName,
		InsecureSkipTLS: true,
		Auth:            auth,
	})
}

func CreateRemoteRepo(emails []string, remoteURL string) error {
	repo, err := createInMemoryRepo(emails)
	if err != nil {
		return err
	}

	repo, err = createRemote(repo, remoteURL)
	if err != nil {
		return err
	}

	return pushInMemoryRepo(repo)
}

func CloneRepo(context context.Context, cloneURL string) (*git.Repository, string, error) {
	dir, err := ioutil.TempDir(os.TempDir(), tempDirPrefix)
	if err != nil {
		return nil, "", err
	}

	repo, err := git.PlainCloneContext(context, dir, true, &git.CloneOptions{
		URL:             cloneURL,
		InsecureSkipTLS: true,
		Auth:            auth,
	})
	if err != nil {
		return nil, "", err
	}

	return repo, dir, nil
}

func CollectMetadata(repo *git.Repository) (CommitMetadata, error) {
	refs, err := repo.References()
	if err != nil {
		return nil, err
	}

	metadata := make(CommitMetadata)
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() == plumbing.HashReference &&
			(ref.Name().IsRemote() || ref.Name().IsBranch()) {
			cIter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
			if err != nil {
				return err
			}
			err = cIter.ForEach(func(c *object.Commit) error {
				metadata[Metadata{Email: c.Author.Email, Name: c.Author.Name}] = struct{}{}
				metadata[Metadata{Email: c.Committer.Email, Name: c.Committer.Name}] = struct{}{}
				return nil
			})
			return err
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return metadata, nil
}

func OpenRepos(paths []string) []OpenResult {
	var repos []OpenResult
	for _, path := range paths {
		repo, err := git.PlainOpen(path)
		repos = append(repos, OpenResult{Origin: path, Repo: repo, Error: err, Dir: path})
	}

	return repos
}

func CloneRepos(urls []string, threads int) <-chan OpenResult {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(threads)

	resultCh := make(chan OpenResult, len(urls))
	urlsCh := make(chan string, len(urls))

	for i := 0; i < threads; i++ {
		go func(ctx context.Context, wg *sync.WaitGroup,
			urlsCh <-chan string,
			resultCh chan<- OpenResult) {
			defer wg.Done()
			for url := range urlsCh {
				repo, dir, err := CloneRepo(ctx, url)
				if ctx.Err() != nil {
					return
				}
				resultCh <- OpenResult{Origin: url, Repo: repo, Dir: dir, Error: err}
			}
		}(ctx, &wg, urlsCh, resultCh)
	}

	for _, url := range urls {
		urlsCh <- url
	}
	close(urlsCh)

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	return resultCh
}

func AnalyseRepos(repos []*git.Repository, threads int) <-chan AnalyseResult {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(threads)

	resultCh := make(chan AnalyseResult, len(repos))
	repoCh := make(chan *git.Repository, len(repos))

	for i := 0; i < threads; i++ {
		go func(ctx context.Context, wg *sync.WaitGroup, repoCh <-chan *git.Repository,
			resultCh chan<- AnalyseResult) {
			defer wg.Done()
			for r := range repoCh {
				if ctx.Err() != nil {
					return
				}
				metadata, err := CollectMetadata(r)
				resultCh <- AnalyseResult{Repo: r, Metadata: metadata, Error: err}
			}
		}(ctx, &wg, repoCh, resultCh)
	}

	for _, r := range repos {
		repoCh <- r
	}
	close(repoCh)

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	return resultCh
}

func SetSSHAuth(username, privateKeyFile, password string) error {
	sshAuth, err := ssh.NewPublicKeysFromFile(username, privateKeyFile, password)
	if err != nil {
		return err
	}

	auth = sshAuth
	return nil
}

func SetBasicAuth(username, password string) {
	auth = &githttp.BasicAuth{
		Username: username,
		Password: password,
	}
}
