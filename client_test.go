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

// verifyURIs is a test helper function that verifies if provided URIs are equal.
func verifyURIs(wanturi, goturi string, t *testing.T) {
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
	wantURI := "/demo.backend.com"
	testFile := "testdata/response_demo_get_upstream.json"

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
	_, err := c.GetStats("demo.backend.com")
	if err != nil {
		t.Fatal(err)
	}

	if !called {
		t.Error("handler not called")
	}
}
