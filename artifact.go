package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/go-github/v34/github"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type artifactList struct {
	Artifacts []artifact `json:"artifacts"`
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
	u, resp, err := client.Actions.DownloadArtifact(ctx, org, repo, a.ID, false)
	if err != nil {
		logrus.Debugf("org: %s, repo: %s, id: %d", org, repo, a.ID)

		e := &ghErr{}
		if resp != nil {
			json.NewDecoder(io.LimitReader(resp.Body, 32*1024)).Decode(e)
		}
		return fmt.Errorf("error getting url to download artifact: %s: %w", e.Message, err)
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
