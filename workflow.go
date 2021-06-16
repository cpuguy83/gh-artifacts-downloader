package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"time"

	"github.com/google/go-github/v34/github"
	"github.com/sirupsen/logrus"
)

func getWorkflowRunArtifacts(ctx context.Context, gh *github.Client, run *github.WorkflowRun, output string) error {
	req, err := http.NewRequest(http.MethodGet, run.GetArtifactsURL(), nil)
	if err != nil {
		return err
	}

	var ls github.ArtifactList
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	resp, err := gh.Do(reqCtx, req, &ls)
	cancel()

	if err != nil {
		return fmt.Errorf("error doing artifacts request: %w", err)
	}
	defer resp.Body.Close()

	if err := checkResponseErr(resp); err != nil {
		return err
	}

	logger(ctx).WithField("numArtfacts", len(ls.Artifacts)).Debug("Got artifact list")

	for _, a := range ls.Artifacts {
		if a.GetExpired() {
			logrus.Debugf("Skipping expired artifact %s", a.Name)
			continue
		}

		if !matcher.MatchString(a.GetName()) {
			logrus.Debugf("Skipping non-matching artifact %s", a.Name)
			continue
		}

		ioutil.WriteFile(filepath.Join(output, "commit"), []byte(run.GetHeadCommit().GetID()), 0600)
		ioutil.WriteFile(filepath.Join(output, "event"), []byte(run.GetEvent()), 0600)
		ioutil.WriteFile(filepath.Join(output, "message"), []byte(run.GetHeadCommit().GetMessage()), 0600)

		if err := getArtifact(ctx, gh, a, output, unpack); err != nil {
			return err
		}
	}

	return nil
}
