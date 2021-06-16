package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-github/v34/github"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	token  = os.Getenv("GITHUB_TOKEN")
	user   = os.Getenv("GITHUB_USER")
	output = os.Getenv("ARTIFACT_OUTPUT")

	unpack  = true
	matcher *regexp.Regexp

	org  string
	repo string
)

func cancelOnSig(ctx context.Context) context.Context {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, signals()...)

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-ch
		cancel()
	}()
	return ctx
}

func main() {
	var (
		pattern        = os.Getenv("ARTIFACT_PATTERN")
		level          = "error"
		workflowIDFl   = os.Getenv("GITHUB_WORKFLOW_ID")
		branchFilter   = os.Getenv("WORKFLOW_BRANCH_FILTER")
		eventFilter    = os.Getenv("WORKFLOW_EVENT_FILTER")
		orgRepo        = os.Getenv("GITHUB_REPO")
		singleArtifact = false
	)

	if pattern == "" {
		pattern = ".*"
	}

	flag.StringVar(&pattern, "pattern", pattern, "regexp pattern to match for artifacts")
	flag.StringVar(&level, "log-level", level, "log level")
	flag.BoolVar(&unpack, "unpack", unpack, "Unpack artifacts")
	flag.StringVar(&output, "output", output, "Directory to output artifacts to. This can also be set by the ARTIFACT_OUTPUT environment variable.")
	flag.StringVar(&orgRepo, "repo", orgRepo, "The repo where artifacts are stored. Use the form <org>/<repository>. This can also be set by the GITHUB_REPO environment variable.")
	flag.StringVar(&workflowIDFl, "id", workflowIDFl, "ID of the last workflow run to get. This can also be set by the GITHUB_WORKFLOW_ID environment variable.")
	flag.StringVar(&branchFilter, "branch-filter", branchFilter, "Filter workflow runs based on branch where the workflow was triggered")
	flag.StringVar(&eventFilter, "event-filter", eventFilter, "Filter workflow runs based on the event that triggered the run")
	flag.BoolVar(&singleArtifact, "single", singleArtifact, "Only download from the workflow run id specified instead of all runs newer than it")

	flag.Parse()

	lvl, err := logrus.ParseLevel(level)
	logrus.SetOutput(os.Stderr)
	errorOut(err, 1)

	if output == "" {
		errorOut(errors.New("must set an output dir"), 1)
	}
	if orgRepo == "" {
		errorOut(errors.New("must set repo flag"), 1)
	}

	wfid, err := strconv.Atoi(workflowIDFl)
	errorOut(errors.Wrap(err, "error parsing workflow id"), 1)

	logrus.SetLevel(lvl)
	logrus.SetOutput(os.Stderr)

	matcher = regexp.MustCompile(pattern)

	httpClient := &http.Client{
		Transport: &basicAuthRT{http.DefaultTransport},
	}
	gh := github.NewClient(httpClient)

	ctx := cancelOnSig(context.Background())

	repoSplit := strings.SplitN(orgRepo, "/", 2)
	org = repoSplit[0]
	repo = repoSplit[1]

	if singleArtifact {
		// TODO
		run, resp, err := gh.Actions.GetWorkflowRunByID(ctx, org, repo, int64(wfid))
		if err != nil {
			errorOut(err, 2)
		}
		defer resp.Body.Close()
		errorOut(checkResponseErr(resp), 2)

		err = getWorkflowRunArtifacts(ctx, gh, run, output)
		errorOut(err, 2)
		return
	}

	var (
		page  int
		count int
		total int
		maxID int64
	)
	defer func() {
		fmt.Println(maxID)
	}()
	for {
		err := func() error {
			if total > 0 && count >= total {
				return fmt.Errorf("%w: ", errDone)
			}
			runs, resp, err := gh.Actions.ListRepositoryWorkflowRuns(ctx, org, repo, &github.ListWorkflowRunsOptions{
				Branch: branchFilter,
				Event:  eventFilter,
				ListOptions: github.ListOptions{
					PerPage: 50,
					Page:    page,
				},
			})
			if err != nil {
				return err
			}
			if err := checkResponseErr(resp); err != nil {
				return err
			}

			defer resp.Body.Close()

			count += len(runs.WorkflowRuns)
			total = runs.GetTotalCount()

			logger(ctx).WithField("page", page).WithField("runs", len(runs.WorkflowRuns)).Debug("Got workflow run batch")
			page = resp.NextPage

			for _, run := range runs.WorkflowRuns {
				if run.GetID() <= int64(wfid) {
					return fmt.Errorf("%w: reached last workflow run id", errDone)
				}

				if run.GetID() > maxID {
					maxID = run.GetID()
				}

				ctx := withLogger(ctx, logrus.WithField("id", run.GetID()))
				if err := getWorkflowRunArtifacts(ctx, gh, run, filepath.Join(output, strconv.Itoa(int(run.GetID())))); err != nil {
					return err
				}
			}

			page++
			return nil
		}()
		if err != nil {
			if !errors.Is(err, errDone) {
				errorOut(err, 2)
			}
			logrus.Debug(err)
			break
		}
	}

}

var errDone = fmt.Errorf("done")

func errorOut(err error, code int) {
	if err == nil {
		return
	}
	if logrus.GetLevel() >= logrus.DebugLevel {
		logrus.Fatalf("%+v", err)
	}
	logrus.Fatal(err)
}
