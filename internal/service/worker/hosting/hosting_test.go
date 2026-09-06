package hosting

import (
	"context"
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"github.com/openmodu/onecatch/internal/service/worker"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

type stubEngine struct{}

func (stubEngine) Available(runtime agentrun.Runtime) bool { return runtime == agentrun.RuntimeCodex }
func (stubEngine) SupportsInteractivePermissions(agentrun.Runtime, agentrun.Sandbox) bool {
	return false
}
func (stubEngine) Run(context.Context, agentrun.Request, agentrun.Sink) (agentrun.Result, error) {
	return agentrun.Result{}, nil
}

func startHost(t *testing.T, root string) *Host {
	t.Helper()
	host := New(Options{Root: root, ID: "desktop", Name: "Test Desktop", Port: freePort(t), Engine: stubEngine{}})
	if _, err := host.Start(context.Background()); err != nil {
		t.Fatalf("start host: %v", err)
	}
	t.Cleanup(func() { _ = host.Stop(context.Background()) })
	return host
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

// A phone pairs with the desktop exactly the way it pairs with the standalone
// worker: one code over TLS, and from then on a pinned certificate.
func TestPairingOverTLSReturnsATokenAndAPinnedCertificate(t *testing.T) {
	host := startHost(t, t.TempDir())
	pairing, err := host.Pair()
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	client := worker.NewClient()
	baseURL := (&url.URL{Scheme: "https", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(host.Status().Port))}).String()
	paired, err := client.Pair(context.Background(), baseURL, pairing.Code)
	if err != nil {
		t.Fatalf("client pair: %v", err)
	}
	if paired.Token == "" || paired.WorkerID != "desktop" {
		t.Fatalf("paired = %+v", paired)
	}
	if paired.ServerCertificateSHA256 != host.Status().Fingerprint {
		t.Fatalf("pinned %q, host reports %q", paired.ServerCertificateSHA256, host.Status().Fingerprint)
	}
	health, err := client.Health(context.Background(), worker.Config{
		BaseURL: baseURL, Token: paired.Token, ServerCertificateSHA256: paired.ServerCertificateSHA256, Enabled: true,
	})
	if err != nil {
		t.Fatalf("health with the pinned certificate: %v", err)
	}
	if !health.Runtimes["codex"] {
		t.Fatalf("health = %+v", health)
	}
}

func TestAPairingCodeIsSingleUse(t *testing.T) {
	host := startHost(t, t.TempDir())
	pairing, err := host.Pair()
	if err != nil {
		t.Fatal(err)
	}
	client := worker.NewClient()
	baseURL := "https://127.0.0.1:" + strconv.Itoa(host.Status().Port)
	if _, err := client.Pair(context.Background(), baseURL, pairing.Code); err != nil {
		t.Fatalf("first pair: %v", err)
	}
	if _, err := client.Pair(context.Background(), baseURL, pairing.Code); err == nil {
		t.Fatal("the same code paired a second device")
	}
}

// Restarting the desktop must not invalidate phones that already paired, so
// the identity behind the pin and the token has to survive on disk.
func TestIdentitySurvivesARestart(t *testing.T) {
	root := filepath.Join(t.TempDir(), "host")
	first := startHost(t, root)
	fingerprint := first.Status().Fingerprint
	if err := first.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := startHost(t, root)
	if second.Status().Fingerprint != fingerprint {
		t.Fatalf("fingerprint changed across restart: %q then %q", fingerprint, second.Status().Fingerprint)
	}
}

func TestStatusOnlyAdvertisesALiveWorker(t *testing.T) {
	host := New(Options{Root: t.TempDir(), Engine: stubEngine{}})
	if status := host.Status(); status.Running || status.Pairing != nil {
		t.Fatalf("idle status = %+v", status)
	}
	if _, err := host.Pair(); err == nil {
		t.Fatal("a stopped host issued a pairing code")
	}
}

func TestLANAddressesSkipLoopbackAndIPv6(t *testing.T) {
	addresses := []net.Addr{
		&net.IPNet{IP: net.ParseIP("127.0.0.1")},
		&net.IPNet{IP: net.ParseIP("192.168.1.20")},
		&net.IPNet{IP: net.ParseIP("192.168.1.20")},
		&net.IPNet{IP: net.ParseIP("fe80::1")},
		&net.IPNet{IP: net.ParseIP("2001:db8::1")},
		&net.IPNet{IP: net.ParseIP("10.0.0.4")},
	}
	got := baseURLsFrom(addresses, 9232)
	want := []string{"https://10.0.0.4:9232", "https://192.168.1.20:9232"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("addresses = %v, want %v", got, want)
	}
}

// The point of hosting the worker on the desktop is that the machine is
// already set up, so a phone should see the projects it already has.
func TestSharedWorkspacesAreServedButNotOwned(t *testing.T) {
	project := t.TempDir()
	shared := []worker.SharedWorkspace{{Mapping: worker.WorkspaceMapping{ID: "onecatch", Name: "OneCatch", Path: project}}}
	host := New(Options{
		Root: t.TempDir(), ID: "desktop", Name: "Test Desktop", Port: freePort(t), Engine: stubEngine{},
		SharedWorkspaces: func(context.Context) ([]worker.SharedWorkspace, error) { return shared, nil },
	})
	if _, err := host.Start(context.Background()); err != nil {
		t.Fatalf("start host: %v", err)
	}
	t.Cleanup(func() { _ = host.Stop(context.Background()) })
	config := pairedConfig(t, host)

	client := worker.NewClient()
	items, err := client.ListWorkspaces(context.Background(), config)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(items) != 1 || items[0].ID != "onecatch" || items[0].Path != project || !items[0].Shared {
		t.Fatalf("workspaces = %+v", items)
	}

	// A phone must not be able to unmap or re-point a project the desktop owns.
	if err := client.RemoveWorkspace(context.Background(), config, "onecatch", false); !isSharedRefusal(err) {
		t.Fatalf("removing a desktop workspace from a phone: %v", err)
	}
	if _, err := client.PrepareWorkspace(context.Background(), config, "onecatch", worker.WorkspacePrepareRequest{Path: project}); !isSharedRefusal(err) {
		t.Fatalf("re-pointing a desktop workspace from a phone: %v", err)
	}

	// Dropping a project on the desktop takes it away from the phone too.
	shared = nil
	host.RefreshSharedWorkspaces(context.Background())
	items, err = client.ListWorkspaces(context.Background(), config)
	if err != nil || len(items) != 0 {
		t.Fatalf("after unsharing: %+v, err = %v", items, err)
	}
}

func pairedConfig(t *testing.T, host *Host) worker.Config {
	t.Helper()
	pairing, err := host.Pair()
	if err != nil {
		t.Fatal(err)
	}
	baseURL := "https://127.0.0.1:" + strconv.Itoa(host.Status().Port)
	paired, err := worker.NewClient().Pair(context.Background(), baseURL, pairing.Code)
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	return worker.Config{BaseURL: baseURL, Token: paired.Token, ServerCertificateSHA256: paired.ServerCertificateSHA256, Enabled: true}
}

func isSharedRefusal(err error) bool {
	var remote worker.RemoteError
	return errors.As(err, &remote) && remote.Code == "worker_workspace_shared"
}

// fakeGit answers the worker's git checks the way the machine holding a Remote
// FS project would: a committed, clean worktree at a known revision.
type fakeGit struct{ head string }

func (g fakeGit) Output(_ context.Context, _ string, args ...string) ([]byte, error) {
	switch strings.Join(args, " ") {
	case "rev-parse --verify HEAD":
		return []byte(g.head + "\n"), nil
	case "ls-files --stage", "status --porcelain=v1 -z --untracked-files=all":
		return nil, nil
	case "config --get remote.origin.url":
		return []byte("git@github.com:openmodu/onecatch.git\n"), nil
	}
	return nil, nil
}

type fakeInspector struct{ snapshot domainworkspaces.GitSnapshot }

func (i fakeInspector) Inspect(context.Context, string) (domainworkspaces.GitSnapshot, error) {
	return i.snapshot, nil
}

type recordingEngine struct{ requests chan agentrun.Request }

func (recordingEngine) Available(agentrun.Runtime) bool { return true }
func (recordingEngine) SupportsInteractivePermissions(agentrun.Runtime, agentrun.Sandbox) bool {
	return false
}
func (e recordingEngine) Run(_ context.Context, request agentrun.Request, _ agentrun.Sink) (agentrun.Result, error) {
	e.requests <- request
	return agentrun.Result{}, nil
}

// A Remote FS project's files are on a third machine. The worker never touches
// this disk for it: git runs over the seam, and so does the agent.
func TestARemoteWorkspaceRunsOnItsOwnMachine(t *testing.T) {
	const head = "1111111111111111111111111111111111111111"
	target := &seam.Target{Host: "build-box", Root: "/srv/onecatch", Username: "ityike"}
	requests := make(chan agentrun.Request, 1)
	host := New(Options{
		Root: t.TempDir(), ID: "desktop", Name: "Test Desktop", Port: freePort(t),
		Engine: recordingEngine{requests: requests},
		SharedWorkspaces: func(context.Context) ([]worker.SharedWorkspace, error) {
			return []worker.SharedWorkspace{{
				Mapping: worker.WorkspaceMapping{ID: "remote-proj", Name: "Remote Project", Path: "/srv/onecatch", RemoteHost: "build-box"},
				Remote:  target,
				Git:     fakeGit{head: head},
				// A local inspector would fail: /srv/onecatch does not exist here.
				Inspector: fakeInspector{snapshot: domainworkspaces.GitSnapshot{IsRepo: true, Head: head, Branch: "main"}},
			}}, nil
		},
	})
	if _, err := host.Start(context.Background()); err != nil {
		t.Fatalf("start host: %v", err)
	}
	t.Cleanup(func() { _ = host.Stop(context.Background()) })
	config := pairedConfig(t, host)
	client := worker.NewClient()

	items, err := client.ListWorkspaces(context.Background(), config)
	if err != nil || len(items) != 1 {
		t.Fatalf("workspaces = %+v, err = %v", items, err)
	}
	if !items[0].Shared || items[0].RemoteHost != "build-box" || items[0].Revision != head {
		t.Fatalf("mapping = %+v", items[0])
	}

	snapshot, err := client.GitStatus(context.Background(), config, "remote-proj")
	if err != nil || snapshot.Branch != "main" || snapshot.Head != head {
		t.Fatalf("git status = %+v, err = %v", snapshot, err)
	}

	if _, err := client.Execute(context.Background(), config, worker.ExecuteRequest{
		RunID: "run-1", WorkspaceID: "remote-proj", Runtime: agentrun.RuntimeCodex,
		Prompt: "look around", Sandbox: agentrun.SandboxReadOnly, BaseRevision: head,
	}, func(agentrun.Event) {}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	request := <-requests
	if request.Remote == nil || request.Remote.Host != "build-box" || request.Remote.Root != "/srv/onecatch" {
		t.Fatalf("the run was not redirected to the workspace's machine: %+v", request.Remote)
	}
	if request.Workspace != "/srv/onecatch" {
		t.Fatalf("workspace = %q", request.Workspace)
	}
}
