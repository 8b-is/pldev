package release

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/peterlodri-sec/pldev/internal/report"
)

// Options controls a release run.
type Options struct {
	Tag       string // e.g. v0.0.2
	Publish   bool   // false = dry-run
	NotesFile string // optional override for the notes body
	Repo      string // owner/name, defaults to peterlodri-sec/pldev
	DistDir   string // defaults to "dist"
}

// Target is one cross-compile output.
type Target struct{ OS, Arch string }

// Targets is the fixed release matrix.
var Targets = []Target{
	{"darwin", "arm64"}, {"darwin", "amd64"},
	{"linux", "arm64"}, {"linux", "amd64"},
}

// Stage seams (overridable in tests).
var (
	buildStage   = buildAll
	signStage    = signArtifact
	verifyStage  = verifyAll
	newPublisher = func(token, repo string) Publisher { return newHTTPPublisher(token, repo) }
)

func defaults(o Options) Options {
	if o.Repo == "" {
		o.Repo = "peterlodri-sec/pldev"
	}
	if o.DistDir == "" {
		o.DistDir = "dist"
	}
	return o
}

// Run is the public entrypoint used by main.go.
func Run(ctx context.Context, rep *report.Reporter, o Options) error {
	return runWith(ctx, rep, execRunner{}, o)
}

func runWith(ctx context.Context, rep *report.Reporter, r Runner, o Options) error {
	o = defaults(o)
	home, _ := os.UserHomeDir()
	edKey := filepath.Join(home, ".ssh", "id_pldev_ci")
	pqKey := filepath.Join(home, ".config", "pldev", "keys", "brett-shaw_pldev_mldsa.key")

	if err := os.MkdirAll(o.DistDir, 0o755); err != nil {
		return fmt.Errorf("dist dir: %w", err)
	}

	// 2. build
	ld, err := gitLdflags(ctx, r, o.Tag)
	if err != nil {
		return err
	}
	rep.Do("build", o.Tag)
	arts, err := buildStage(ctx, r, ld, o.DistDir)
	if err != nil {
		rep.Fail("build", o.Tag, err)
		return err
	}
	rep.OK("build", o.Tag, report.KV{K: "n", V: fmt.Sprint(len(arts))})

	// 3. sign
	for _, a := range arts {
		rep.Do("sign", a)
		if err := signStage(ctx, r, a, edKey, pqKey); err != nil {
			rep.Fail("sign", a, err)
			return err
		}
		rep.OK("sign", a)
	}

	// 4. verify gate
	if err := verifyStage(ctx, rep, arts); err != nil {
		rep.Fail("verify", o.Tag, err)
		return err
	}
	rep.OK("verify", o.Tag)

	// 5. notes
	prev := prevTag(ctx, r)
	body := o.NotesFile
	if body != "" {
		b, err := os.ReadFile(o.NotesFile)
		if err != nil {
			return fmt.Errorf("notes file: %w", err)
		}
		body = string(b)
	} else {
		log, err := gitLog(ctx, r, prev)
		if err != nil {
			return err
		}
		body = renderNotes(log)
	}
	prov, err := renderProvenance(arts)
	if err != nil {
		return err
	}
	notes := body + "\n" + prov

	if !o.Publish {
		rep.Info("dry-run complete — artifacts signed in " + o.DistDir)
		fmt.Println(notes)
		rep.Skip("publish", o.Tag, "dry-run (pass --publish)")
		return nil
	}

	// publish: token -> triple-ack gate -> upload
	tok, err := resolveToken(ctx, r, os.Getenv)
	if err != nil {
		return err
	}
	// Network destination, always shown to the operator before they ack.
	releasesURL := shortURL(githubAPIBase + "/repos/" + o.Repo + "/releases")
	if err := confirmPublish(o.Tag, releasesURL, arts); err != nil {
		rep.Skip("publish", o.Tag, err.Error())
		return err
	}
	pub := newPublisher(tok, o.Repo)
	rep.Info("creating release", report.KV{K: "to", V: releasesURL})
	up, htmlURL, err := pub.CreateRelease(ctx, releaseReq{Tag: o.Tag, Name: o.Tag, Body: notes})
	if err != nil {
		rep.Fail("publish", o.Tag, err, report.KV{K: "to", V: releasesURL})
		return err
	}
	uploadTo := shortURL(up)
	rep.Info("uploading assets", report.KV{K: "to", V: uploadTo})
	for _, a := range arts {
		for _, p := range []string{a, a + ".sig", a + ".sig.mldsa"} {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if err := pub.UploadAsset(ctx, up, filepath.Base(p), data); err != nil {
				rep.Fail("upload", p, err, report.KV{K: "to", V: uploadTo}, report.KV{K: "release", V: htmlURL})
				return fmt.Errorf("upload %s (release exists at %s): %w", p, htmlURL, err)
			}
			rep.OK("upload", filepath.Base(p), report.KV{K: "to", V: uploadTo})
		}
	}
	rep.OK("publish", o.Tag, report.KV{K: "url", V: htmlURL})
	return nil
}
