package nginxhealthz_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	nginxhealthz "github.com/qba73/nginx-healthz"
)

func newTestServerWithPathValidator(testFile string, wantURI string, t *testing.T) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		gotReqURI := r.RequestURI
		verifyURIs(wantURI, gotReqURI, t)

		f, err := os.Open(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		_, err = io.Copy(rw, f)
		if err != nil {
			t.Fatalf("copying data from file %s to test HTTP server: %v", testFile, err)
		}
	}))
	return ts
}

// verifyURIs is a test helper function that verifies if provided URIs are equal.
func verifyURIs(wanturi, goturi string, t *testing.T) {
	t.Helper()

	wantU, err := url.Parse(wanturi)
	if err != nil {
		t.Fatalf("error parsing URL %q, %v", wanturi, err)
	}
	gotU, err := url.Parse(goturi)
	if err != nil {
		t.Fatalf("error parsing URL %q, %v", wanturi, err)
	}

	if !cmp.Equal(wantU.Path, gotU.Path) {
		t.Fatalf(cmp.Diff(wantU.Path, gotU.Path))
	}

	wantQuery, err := url.ParseQuery(wantU.RawQuery)
	if err != nil {
		t.Fatal(err)
	}
	gotQuery, err := url.ParseQuery(gotU.RawQuery)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(wantQuery, gotQuery) {
		t.Fatalf("URIs are not equal, \n%s", cmp.Diff(wantQuery, gotQuery))
	}
}

func TestNewClient_FailsOnInvalidVersion(t *testing.T) {
	t.Parallel()

	_, err := nginxhealthz.NewClient(
		"http://localhost:9001",
		nginxhealthz.WithVersion(9),
	)
	if err == nil {
		t.Fatal("want err on invalid NGINX version")
	}
}

func TestNewClient_FailsOnInvalidBaseURL(t *testing.T) {
	t.Parallel()

	_, err := nginxhealthz.NewClient("")
	if err == nil {
		t.Fatal("want error on invalid base URL")
	}
}

func TestClientCallsValidPath(t *testing.T) {
	t.Parallel()

	var called bool
	wantURI := "/api/8/http/upstreams/demo-backend"
	testFile := "testdata/response_get_upstream_all_servers_up.json"

	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		gotReqURI := r.RequestURI
		verifyURIs(wantURI, gotReqURI, t)

		f, err := os.Open(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		_, err = io.Copy(rw, f)
		if err != nil {
			t.Fatalf("copying data from file %s to test HTTP server: %v", testFile, err)
		}
		called = true
	}))
	defer ts.Close()

	c, err := nginxhealthz.NewClient(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.GetStatsFor("demo-backend")
	if err != nil {
		t.Fatal(err)
	}

	if !called {
		t.Error("handler not called")
	}
}

func TestClientGetsStatsOnValidInputWithAllServersUp(t *testing.T) {
	t.Parallel()

	ts := newTestServerWithPathValidator(
		"testdata/response_get_upstream_all_servers_up.json",
		"/api/8/http/upstreams/demo-backend", t,
	)
	defer ts.Close()

	c, err := nginxhealthz.NewClient(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	got, err := c.GetStatsFor("demo-backend")
	if err != nil {
		t.Error(err)
	}

	want := nginxhealthz.Stats{
		Total: 2,
		Up:    2,
		Down:  0,
	}

	if !cmp.Equal(want, got) {
		t.Error(cmp.Diff(want, got))
	}

}

func TestClientGetsUpstreamsForHostnameOnValidInput(t *testing.T) {
	t.Parallel()

	ts := newTestServerWithPathValidator(
		"testdata/response_get_upstreams_zones.json",
		"/api/8/http/upstreams?fields=zone", t)
	defer ts.Close()

	c, err := nginxhealthz.NewClient(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	got, err := c.GetUpstreamsFor("bar.example.org")
	if err != nil {
		t.Fatal(err)
	}

	want := map[string][]string{"bar.example.org": {"hg-backend", "lxr-backend"}}

	if !cmp.Equal(want, got, cmpopts.SortSlices(func(x, y string) bool { return x < y })) {
		t.Error(cmp.Diff(want, got))
	}
}

func TestGetStatsForHost_ReturnsCorrectResultsForValidHost(t *testing.T) {
	t.Parallel()

	h := func(testFile string, w http.ResponseWriter, r *http.Request, t *testing.T) {
		f, err := os.Open(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		if err != nil {
			t.Fatalf("copying data from file %s to test HTTP server: %v", testFile, err)
		}
	}

	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "lxr-backend") {
			testFile := "testdata/response_get_upstream_lxr_backend.json"
			h(testFile, rw, r, t)
		} else {
			testFile := "testdata/response_get_upstream_hg_backend.json"
			h(testFile, rw, r, t)
		}
	}))
	defer ts.Close()

	c, err := nginxhealthz.NewClient(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	got := c.GetStatsForUpstreams([]string{"hg-backend", "lxr-backend"})

	want := nginxhealthz.Stats{
		Total: 4,
		Up:    3,
		Down:  1,
	}

	if !cmp.Equal(want, got) {
		t.Error(cmp.Diff(want, got))
	}
}
