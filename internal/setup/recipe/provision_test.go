package recipe

import (
	"errors"
	"testing"
)

type fakeAuth struct {
	called *bool
	err    error
}

func (f fakeAuth) Authenticate(string) error {
	if f.called != nil {
		*f.called = true
	}
	return f.err
}

type fakeRemote struct {
	host   *string
	argv   *[]string
	called *bool
	err    error
}

func (f fakeRemote) Run(host string, argv []string) (string, error) {
	if f.called != nil {
		*f.called = true
	}
	if f.host != nil {
		*f.host = host
	}
	if f.argv != nil {
		*f.argv = argv
	}
	return "", f.err
}

func TestProvisionSatisfiedWhenHealthy(t *testing.T) {
	remoteCalled := false
	s := Provision{
		Name:        "stack",
		HealthProbe: func(Context) bool { return true },
		Auth:        fakeAuth{},
		Remote:      fakeRemote{called: &remoteCalled},
	}
	if st, _ := s.Check(ctx()); st != StatusSatisfied {
		t.Error("healthy stack should be satisfied")
	}
	if remoteCalled {
		t.Error("Check must not run the remote command")
	}
}

func TestProvisionAuthThenRemote(t *testing.T) {
	authed, remoteCalled := false, false
	var gotHost string
	var gotArgv []string
	s := Provision{
		Name:        "stack",
		Host:        "public-services",
		Argv:        []string{"docker", "compose", "up", "-d"},
		HealthProbe: func(Context) bool { return false },
		Auth:        fakeAuth{called: &authed},
		Remote:      fakeRemote{called: &remoteCalled, host: &gotHost, argv: &gotArgv},
	}
	if st, _ := s.Check(ctx()); st != StatusPending {
		t.Fatal("down stack should be pending")
	}
	if err := s.Apply(ctx()); err != nil {
		t.Fatal(err)
	}
	if !authed || !remoteCalled {
		t.Errorf("expected auth+remote, got authed=%v remote=%v", authed, remoteCalled)
	}
	if gotHost != "public-services" || len(gotArgv) != 4 {
		t.Errorf("remote got host=%q argv=%v", gotHost, gotArgv)
	}
}

func TestProvisionAuthFailureBlocksRemote(t *testing.T) {
	remoteCalled := false
	s := Provision{
		Name:        "stack",
		HealthProbe: func(Context) bool { return false },
		Auth:        fakeAuth{err: errors.New("touch id declined")},
		Remote:      fakeRemote{called: &remoteCalled},
	}
	if err := s.Apply(ctx()); err == nil {
		t.Error("auth failure must error")
	}
	if remoteCalled {
		t.Error("remote must not run when auth fails")
	}
}

func TestProvisionRequiresAuthAndRemote(t *testing.T) {
	s := Provision{Name: "stack", HealthProbe: func(Context) bool { return false }}
	if err := s.Apply(ctx()); err == nil {
		t.Error("missing auth/remote must error")
	}
}
