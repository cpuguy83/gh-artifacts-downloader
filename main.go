package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-github/v34/github"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	token = os.Getenv("GITHUB_TOKEN")
	user  = os.Getenv("GITHUB_USER")
)

func main() {
	var (
		pattern      = os.Getenv("ARTIFACT_PATTERN")
		level        = "debug"
		unpack       = true
		output       = os.Getenv("ARTIFACT_OUTPUT")
		repo         = os.Getenv("GITHUB_REPO")
		workflowIDFl = os.Getenv("GITHUB_WORKFLOW_ID")
	)

	if pattern == "" {
		pattern = ".*"
	}

	flag.StringVar(&pattern, "pattern", pattern, "regexp pattern to match for artifacts")
	flag.StringVar(&level, "log-level", level, "log level")
	flag.BoolVar(&unpack, "unpack", unpack, "Unpack artifacts")
	flag.StringVar(&output, "output", output, "Directory to output artifacts to. This can also be set by the ARTIFACT_OUTPUT environment variable.")
	flag.StringVar(&repo, "repo", repo, "The repo where artifacts are stored. Use the form <org>/<repository>. This can also be set by the GITHUB_REPO environment variable.")
	flag.StringVar(&workflowIDFl, "id", workflowIDFl, "ID of the workflow you want to get artifacts from. This can also be set by the GITHUB_WORKFLOW_ID environment variable.")

	flag.Parse()

	lvl, err := logrus.ParseLevel(level)
	errorOut(err, 1)

	if repo == "" {
		errorOut(errors.New("must set repo flag"), 1)
	}

	wfid, err := strconv.Atoi(workflowIDFl)
	errorOut(errors.Wrap(err, "error parsing workflow id"), 1)

	logrus.SetLevel(lvl)
	logrus.SetOutput(os.Stderr)

	matcher := regexp.MustCompile(pattern)

	gh := github.NewClient(&http.Client{
		Transport: &basicAuthRT{http.DefaultTransport},
	})

	ctx := context.TODO()

	repoSplit := strings.SplitN(repo, "/", 2)
	w, ghResp, err := gh.Actions.GetWorkflowRunByID(ctx, repoSplit[0], repoSplit[1], int64(wfid))
	errorOut(err, 1)

	ghResp.Body.Close()

	resp, err := http.Get(w.GetArtifactsURL())
	if err != nil {
		errorOut(err, 2)
	}
	defer resp.Body.Close()

	type al struct {
		Artifacts []artifact `json:"artifacts"`
	}

	var ls al
	if err := json.NewDecoder(resp.Body).Decode(&ls); err != nil {
		errorOut(err, 2)
	}

	for _, a := range ls.Artifacts {
		if a.Expired {
			logrus.Debugf("Skipping expired artifact %s", a.Name)
			continue
		}

		if !matcher.MatchString(a.Name) {
			logrus.Debugf("Skipping non-matching artifact %s, pattern %s", a.Name, pattern)
			continue
		}

		if output == "" {
			fmt.Println(a.URL)
			continue
		}

		a.org = repoSplit[0]
		a.repo = repoSplit[1]

		if err := getArtifact(ctx, gh, a, output, unpack); err != nil {
			errorOut(err, 2)
		}
	}
}

type artifact struct {
	ID      int64  `json:"id"`
	URL     string `json:"archive_download_url"`
	Expired bool   `json:"expired"`
	Name    string `json:"name"`
	Size    int64  `json:"size_in_bytes"`
	org     string
	repo    string
}

func getArtifact(ctx context.Context, client *github.Client, a artifact, dir string, unpack bool) error {
	u, resp, err := client.Actions.DownloadArtifact(ctx, a.org, a.repo, a.ID, false)
	if err != nil {
		logrus.Debugf("org: %s, repo: %s, id: %d", a.org, a.repo, a.ID)
		type ghErr struct {
			Message string `json:"message"`
		}
		e := &ghErr{}
		json.NewDecoder(io.LimitReader(resp.Body, 32*1024)).Decode(e)
		return errors.Wrapf(err, "error getting url to download artifact: %s", e.Message)
	}
	resp.Body.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.Wrap(err, "error creating artifact dir")
	}

	f, err := os.OpenFile(filepath.Join(dir, a.Name)+".zip", os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	if err != nil {
		return errors.Wrap(err, "error creating save file")
	}
	defer f.Close()

	resp, err = client.Do(ctx, req, f)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if !unpack {
		return nil
	}

	// Seek here because I've found the size reported by github to be unreliable.
	pos, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return errors.Wrap(err, "error getting artifact size")
	}

	r, err := zip.NewReader(f, pos)
	if err != nil {
		return errors.Wrapf(err, "error making zip reader from file %s", f.Name())
	}
	if err := unzip(r, a, dir); err != nil {
		return err
	}

	f.Close()
	if err := os.Remove(f.Name()); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

var _ http.RoundTripper = basicAuthRT{}

type basicAuthRT struct {
	rt http.RoundTripper
}

func (t basicAuthRT) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.SetBasicAuth(user, token)
	return t.rt.RoundTrip(req)
}

func unzip(r *zip.Reader, a artifact, dest string) error {
	for _, zf := range r.File {
		err := func() error {
			rc, err := zf.Open()
			if err != nil {
				return errors.Wrap(err, "error opening file in zip")
			}
			defer rc.Close()

			if zf.Mode().IsDir() {
				if err := os.MkdirAll(filepath.Join(dest, a.Name, zf.Name), 0755); err != nil {
					return err
				}
			} else {
				if parent := filepath.Dir(filepath.Join(dest, a.Name, zf.Name)); parent != "" {
					if err := os.MkdirAll(parent, 0755); err != nil {
						return errors.Wrap(err, "error creating parent dir for file in zip")
					}
				}

				f, err := os.OpenFile(filepath.Join(dest, a.Name, zf.Name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, zf.Mode().Perm())
				if err != nil {
					return errors.Wrap(err, "error creating file for unpacked result")
				}
				defer f.Close()
				if _, err := io.Copy(f, rc); err != nil {
					return err
				}
			}

			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

func errorOut(err error, code int) {
	if err == nil {
		return
	}
	if logrus.GetLevel() >= logrus.DebugLevel {
		logrus.Fatalf("%+v", err)
	}
	logrus.Fatal(err)
}
