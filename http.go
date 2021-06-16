package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/go-github/v34/github"
)

type ghErr struct {
	Message string `json:"message"`
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

func checkResponseErr(resp *github.Response) error {
	if resp.StatusCode < 400 {
		return nil
	}

	var m ghErr
	json.NewDecoder(io.LimitReader(resp.Body, 16*1024)).Decode(&m)
	return fmt.Errorf("StatusCode: %d, Message: %s", resp.StatusCode, m.Message)
}
