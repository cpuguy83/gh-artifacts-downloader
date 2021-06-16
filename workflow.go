package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/go-github/v34/github"
	"github.com/sirupsen/logrus"
)

func getWorkflowRunArtifacts(ctx context.Context, gh *github.Client, run *github.WorkflowRun, output string) error {
	req, err := http.NewRequest(http.MethodGet, run.GetArtifactsURL(), nil)
	if err != nil {
		return err
	}

	var ls artifactList
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
		if a.Expired {
			logrus.Debugf("Skipping expired artifact %s", a.Name)
			continue
		}

		if !matcher.MatchString(a.Name) {
			logrus.Debugf("Skipping non-matching artifact %s", a.Name)
			continue
		}

		if err := getArtifact(ctx, gh, a, output, unpack); err != nil {
			return err
		}
	}

	return nil
}
