package nginxhealthz_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"

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

func TestClientCallsValidPath(t *testing.T) {
	t.Parallel()

	var called bool
	wantURI := "/api/8/http"
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

	c := nginxhealthz.NewClient(ts.URL)
	_, err := c.GetStats()
	if err != nil {
		t.Fatal(err)
	}

	if !called {
		t.Error("handler not called")
	}
}

func TestClientGetsStatsOnValidInputWithAllServersUp(t *testing.T) {
	t.Parallel()

	ts := newTestServerWithPathValidator("testdata/response_get_upstream_all_servers_up.json", "/api/8/http", t)
	defer ts.Close()

	c := nginxhealthz.NewClient(ts.URL)
	got, err := c.GetStats()
	if err != nil {
		t.Error(err)
	}

	want := nginxhealthz.UpstreamStatus{
		Total: 2,
		Up:    2,
		Down:  0,
	}

	if !cmp.Equal(want, got) {
		t.Error(cmp.Diff(want, got))
	}

}

func TestClientRetrievesZonesOnValidInput(t *testing.T) {
	t.Parallel()

	ts := newTestServerWithPathValidator("testdata/response_get_upstreams_zones.json", "/api/8/http/upstreams?fields=zone", t)
	defer ts.Close()

	c := nginxhealthz.NewClient(ts.URL)
	got, err := c.GetZones()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"foo.example.com-demo-backend",
		"bar.example.com-trac-backend",
		"foo.example.org-hg-backend",
		"bar.example.org-lxr-backend",
	}

	if !cmp.Equal(want, got) {
		t.Error(cmp.Diff(want, got))
	}
}
