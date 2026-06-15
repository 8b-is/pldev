package release

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPPublisher(t *testing.T) {
	var createdBody string
	var uploadedName, uploadedData string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases") && r.Method == http.MethodPost:
			buf := new(strings.Builder)
			_, _ = io.Copy(buf, r.Body)
			createdBody = buf.String()
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"upload_url":"` + uploadBase(r) + `{?name,label}","html_url":"http://x/rel"}`))
		case strings.Contains(r.URL.Path, "/assets") && r.Method == http.MethodPost:
			uploadedName = r.URL.Query().Get("name")
			buf := new(strings.Builder)
			_, _ = io.Copy(buf, r.Body)
			uploadedData = buf.String()
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	p := &httpPublisher{token: "t", apiBase: srv.URL, repo: "owner/repo", client: srv.Client()}
	up, htmlURL, err := p.CreateRelease(context.Background(), releaseReq{Tag: "v0.0.2", Name: "v0.0.2", Body: "notes"})
	if err != nil {
		t.Fatal(err)
	}
	if htmlURL != "http://x/rel" {
		t.Errorf("html url: %s", htmlURL)
	}
	if !strings.Contains(createdBody, `"tag_name":"v0.0.2"`) {
		t.Errorf("create body: %s", createdBody)
	}
	if err := p.UploadAsset(context.Background(), up, "pldev-linux-arm64", []byte("BINDATA")); err != nil {
		t.Fatal(err)
	}
	if uploadedName != "pldev-linux-arm64" || uploadedData != "BINDATA" {
		t.Errorf("upload name=%q data=%q", uploadedName, uploadedData)
	}
}

// uploadBase returns the server origin + /assets path for the test.
func uploadBase(r *http.Request) string {
	return "http://" + r.Host + "/repos/owner/repo/releases/1/assets"
}
