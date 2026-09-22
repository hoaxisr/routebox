package updates

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const amneziaFixture = `{
  "tag_name": "v1.12.9-awg2.0",
  "body": "## sing-box with AWG 2.0 support\n- H1-H4 ranges",
  "published_at": "2026-06-01T10:00:00Z",
  "assets": [
    {"name": "sing-box-1.12.9-awg2.0-linux-amd64", "browser_download_url": "https://dl/sing-box-1.12.9-awg2.0-linux-amd64"},
    {"name": "sing-box-1.12.9-awg2.0-entware-mipsel", "browser_download_url": "https://dl/sing-box-1.12.9-awg2.0-entware-mipsel"},
    {"name": "sing-box-1.12.9-awg2.0-entware-aarch64", "browser_download_url": "https://dl/sing-box-1.12.9-awg2.0-entware-aarch64"},
    {"name": "checksums.txt", "browser_download_url": "https://dl/checksums.txt"}
  ]
}`

const routeboxFixture = `{
  "tag_name": "v0.18.0",
  "body": "Bug fixes",
  "published_at": "2026-06-02T10:00:00Z",
  "assets": [
    {"name": "routebox-linux-amd64", "browser_download_url": "https://dl/routebox-linux-amd64"},
    {"name": "routebox-linux-amd64.sha256", "browser_download_url": "https://dl/routebox-linux-amd64.sha256"}
  ]
}`

func fixtureServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			body = "[" + body + "]"
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestChecker(t *testing.T, base, arch string) *Checker {
	t.Helper()
	t.Setenv("UPDATES_API_BASE", base)
	c := NewChecker()
	c.arch = arch
	return c
}

func amneziaTestTarget() Target {
	return Target{
		Name: "amnezia-box",
		Repo: "hoaxisr/amnezia-box",
		AssetSuffix: func(arch string) (string, bool) {
			switch arch {
			case "amd64":
				return "linux-amd64", true
			case "mipsle":
				return "entware-mipsel", true
			case "arm64":
				return "entware-aarch64", true
			}
			return "", false
		},
	}
}

func TestCheckParsesTagNotesAndDate(t *testing.T) {
	srv := fixtureServer(t, http.StatusOK, amneziaFixture)
	c := newTestChecker(t, srv.URL, "amd64")

	info, err := c.Check(amneziaTestTarget())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if info.Version != "1.12.9" {
		t.Errorf("Version = %q, want 1.12.9 (v prefix and -awg2.0 stripped)", info.Version)
	}
	if info.TagName != "v1.12.9-awg2.0" {
		t.Errorf("TagName = %q", info.TagName)
	}
	if !strings.Contains(info.Notes, "AWG 2.0") {
		t.Errorf("Notes = %q", info.Notes)
	}
	if info.PublishedAt.IsZero() {
		t.Error("PublishedAt not parsed")
	}
	cached, ok := c.Cached("amnezia-box")
	if !ok || cached.Release == nil || cached.Release.Version != "1.12.9" {
		t.Errorf("cache not populated: %+v ok=%v", cached, ok)
	}
}

func TestCheckSelectsAssetByArch(t *testing.T) {
	cases := []struct{ arch, wantAsset string }{
		{"amd64", "sing-box-1.12.9-awg2.0-linux-amd64"},
		{"mipsle", "sing-box-1.12.9-awg2.0-entware-mipsel"},
		{"arm64", "sing-box-1.12.9-awg2.0-entware-aarch64"},
	}
	for _, tc := range cases {
		srv := fixtureServer(t, http.StatusOK, amneziaFixture)
		c := newTestChecker(t, srv.URL, tc.arch)
		info, err := c.Check(amneziaTestTarget())
		if err != nil {
			t.Fatalf("%s: %v", tc.arch, err)
		}
		if info.AssetName != tc.wantAsset {
			t.Errorf("%s: AssetName = %q, want %q", tc.arch, info.AssetName, tc.wantAsset)
		}
		if info.Sha256URL != "https://dl/checksums.txt" {
			t.Errorf("%s: Sha256URL = %q, want checksums.txt URL", tc.arch, info.Sha256URL)
		}
	}
}

func TestCheckUnsupportedArch(t *testing.T) {
	srv := fixtureServer(t, http.StatusOK, amneziaFixture)
	c := newTestChecker(t, srv.URL, "riscv64")
	if _, err := c.Check(amneziaTestTarget()); err == nil {
		t.Fatal("want error for unsupported arch")
	}
}

func TestCheckSha256AssetForm(t *testing.T) {
	srv := fixtureServer(t, http.StatusOK, routeboxFixture)
	c := newTestChecker(t, srv.URL, "amd64")
	target := Target{
		Name: "routebox",
		Repo: "hoaxisr/routebox",
		AssetSuffix: func(arch string) (string, bool) {
			if arch == "amd64" {
				return "routebox-linux-amd64", true
			}
			return "", false
		},
	}
	info, err := c.Check(target)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if info.AssetName != "routebox-linux-amd64" {
		t.Errorf("AssetName = %q (must not pick the .sha256 asset)", info.AssetName)
	}
	if info.Sha256URL != "https://dl/routebox-linux-amd64.sha256" {
		t.Errorf("Sha256URL = %q, want <asset>.sha256 form", info.Sha256URL)
	}
}

func TestCheck404NoReleases(t *testing.T) {
	srv := fixtureServer(t, http.StatusNotFound, `{"message":"Not Found"}`)
	c := newTestChecker(t, srv.URL, "amd64")
	_, err := c.Check(amneziaTestTarget())
	if err == nil || !strings.Contains(err.Error(), "no releases") {
		t.Fatalf("err = %v, want 'no releases'", err)
	}
	cached, _ := c.Cached("amnezia-box")
	if cached.Error == "" {
		t.Error("cache must record the error")
	}
}

// Every fork release since the 1.15 rebase is a GitHub prerelease (gh marks
// -alpha/-beta tags as such), and /releases/latest skips prereleases → 404.
// The checker must pick the newest published non-draft release from the list,
// prerelease or not, ignoring list order.
func TestCheckPicksNewestPublishedIncludingPrerelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`[
  {"tag_name": "1.15.0-alpha.6-awgm.24", "prerelease": true, "draft": false, "published_at": "2026-09-21T16:03:30Z",
   "assets": [{"name": "singbox-1.15.0-alpha.6-awgm.24-linux-amd64", "browser_download_url": "https://dl/24"},
              {"name": "singbox-1.15.0-alpha.6-awgm.24-linux-amd64.sha256", "browser_download_url": "https://dl/24.sha256"}]},
  {"tag_name": "1.15.0-alpha.7-awgm.26", "prerelease": true, "draft": true, "published_at": "2026-09-23T06:07:15Z",
   "assets": [{"name": "singbox-1.15.0-alpha.7-awgm.26-linux-amd64", "browser_download_url": "https://dl/26"}]},
  {"tag_name": "1.15.0-alpha.6-awgm.25", "prerelease": true, "draft": false, "published_at": "2026-09-22T06:07:15Z",
   "assets": [{"name": "singbox-1.15.0-alpha.6-awgm.25-linux-amd64", "browser_download_url": "https://dl/25"},
              {"name": "singbox-1.15.0-alpha.6-awgm.25-linux-amd64.sha256", "browser_download_url": "https://dl/25.sha256"}]}
]`))
	}))
	t.Cleanup(srv.Close)
	c := newTestChecker(t, srv.URL, "amd64")
	info, err := c.Check(amneziaTestTarget())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if info.TagName != "1.15.0-alpha.6-awgm.25" {
		t.Errorf("TagName = %q, want newest published non-draft 1.15.0-alpha.6-awgm.25", info.TagName)
	}
	if info.Sha256URL != "https://dl/25.sha256" {
		t.Errorf("Sha256URL = %q", info.Sha256URL)
	}
}

func TestCheckEmptyListNoReleases(t *testing.T) {
	srv := fixtureServer(t, http.StatusOK, "")
	c := newTestChecker(t, srv.URL, "amd64")
	_, err := c.Check(amneziaTestTarget())
	if err == nil || !strings.Contains(err.Error(), "no releases") {
		t.Fatalf("err = %v, want 'no releases'", err)
	}
}

func TestCheck403RateLimit(t *testing.T) {
	srv := fixtureServer(t, http.StatusForbidden, `{"message":"API rate limit exceeded"}`)
	c := newTestChecker(t, srv.URL, "amd64")
	_, err := c.Check(amneziaTestTarget())
	if err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("err = %v, want rate-limit message", err)
	}
}

func TestNormalizeVersion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"v1.12.9-awg2.0", "1.12.9"},
		{"1.12.9", "1.12.9"},
		{"1.14.0-alpha.48-awg3-xhttp-mieru-4", "1.14.0-alpha.48-awg3-xhttp-mieru-4"},
	}
	for _, tc := range cases {
		if got := NormalizeVersion(tc.in); got != tc.want {
			t.Errorf("NormalizeVersion(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
