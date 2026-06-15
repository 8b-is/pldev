package release

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// githubAPIBase is the production GitHub REST endpoint.
const githubAPIBase = "https://api.github.com"

// shortURL renders a network target for display: host+path only, no scheme,
// no query string. Secrets live in headers, never in the URL, so nothing to
// mask here — this just keeps the destination visible and concise.
func shortURL(u string) string {
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	if i := strings.IndexByte(u, '?'); i >= 0 {
		u = u[:i]
	}
	return u
}

// Publisher uploads a release to GitHub.
type Publisher interface {
	CreateRelease(ctx context.Context, req releaseReq) (uploadURL, htmlURL string, err error)
	UploadAsset(ctx context.Context, uploadURL, name string, data []byte) error
}

type releaseReq struct {
	Tag  string
	Name string
	Body string
}

type httpPublisher struct {
	token   string
	apiBase string // https://api.github.com
	repo    string // owner/name
	client  *http.Client
}

// newHTTPPublisher builds a publisher with a TLS 1.2-min hardened client.
func newHTTPPublisher(token, repo string) *httpPublisher {
	tr := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	return &httpPublisher{
		token:   token,
		apiBase: githubAPIBase,
		repo:    repo,
		client:  &http.Client{Timeout: 60 * time.Second, Transport: tr},
	}
}

func (p *httpPublisher) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return p.client.Do(req)
}

func (p *httpPublisher) CreateRelease(ctx context.Context, r releaseReq) (string, string, error) {
	payload, _ := json.Marshal(map[string]string{
		"tag_name": r.Tag, "name": r.Name, "body": r.Body,
	})
	url := fmt.Sprintf("%s/repos/%s/releases", p.apiBase, p.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return "", "", fmt.Errorf("create release: %s: %s", resp.Status, body)
	}
	var out struct {
		UploadURL string `json:"upload_url"`
		HTMLURL   string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", "", err
	}
	// strip the RFC6570 template suffix: ".../assets{?name,label}"
	up := out.UploadURL
	if i := strings.IndexByte(up, '{'); i >= 0 {
		up = up[:i]
	}
	return up, out.HTMLURL, nil
}

func (p *httpPublisher) UploadAsset(ctx context.Context, uploadURL, name string, data []byte) error {
	url := uploadURL + "?name=" + name
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := p.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload %s: %s: %s", name, resp.Status, b)
	}
	return nil
}
