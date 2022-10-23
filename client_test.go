package nginxhealthz_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	nginxhealthz "github.com/qba73/nginx-healthz"
)

func newTestServerWithPathValidator(respBody string, wantURI string, t *testing.T) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		gotReqURI := r.RequestURI
		verifyURIs(wantURI, gotReqURI, t)

		_, err := io.WriteString(rw, respBody)
		if err != nil {
			t.Fatal(err)
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

	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		gotReqURI := r.RequestURI
		verifyURIs(wantURI, gotReqURI, t)

		_, err := io.WriteString(rw, validResponseGetUpstreamAllServersUp)
		if err != nil {
			t.Fatal(err)
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
		validResponseGetUpstreamAllServersUp,
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
		validResponseGetUpstreamsZones,
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

	h := func(responseBody string, w http.ResponseWriter, r *http.Request, t *testing.T) {
		t.Log(r.URL.Path)
		_, err := io.WriteString(w, responseBody)
		if err != nil {
			t.Fatal(err)
		}
	}

	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "lxr-backend") {
			h(validResponseUpstreamLXRbackend, rw, r, t)
		} else {
			h(validResponseUpstreamHGbackend, rw, r, t)
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

var (
	validResponseUpstreamLXRbackend = `{
		"peers": [
			{
				"id": 0,
				"server": "10.0.0.42:8084",
				"name": "10.0.0.42:8084",
				"backup": false,
				"weight": 1,
				"state": "up",
				"active": 1,
				"ssl": {
					"handshakes": 19803694,
					"handshakes_failed": 0,
					"session_reuses": 19803274
				},
				"requests": 19803806,
				"header_time": 10,
				"response_time": 10,
				"responses": {
					"1xx": 0,
					"2xx": 17210560,
					"3xx": 8364,
					"4xx": 1721,
					"5xx": 0,
					"codes": {
						"200": 17210560,
						"301": 7922,
						"304": 442,
						"400": 1,
						"404": 1086,
						"405": 634
					},
					"total": 17220645
				},
				"sent": 7525574329,
				"received": 16975650045,
				"fails": 2583043,
				"unavail": 0,
				"health_checks": {
					"checks": 2852546,
					"fails": 0,
					"unhealthy": 0,
					"last_passed": true
				},
				"downtime": 0,
				"selected": "2022-10-17T20:38:40Z"
			},
			{
				"id": 1,
				"server": "10.0.0.41:8084",
				"name": "10.0.0.41:8084",
				"backup": false,
				"weight": 1,
				"state": "up",
				"active": 0,
				"ssl": {
					"handshakes": 21808054,
					"handshakes_failed": 0,
					"session_reuses": 21807458
				},
				"requests": 21808225,
				"header_time": 11,
				"response_time": 11,
				"responses": {
					"1xx": 0,
					"2xx": 19223635,
					"3xx": 275,
					"4xx": 1089,
					"5xx": 0,
					"codes": {
						"200": 19223634,
						"206": 1,
						"301": 100,
						"304": 175,
						"404": 623,
						"405": 466
					},
					"total": 19224999
				},
				"sent": 10110237497,
				"received": 18490496811,
				"fails": 2583040,
				"unavail": 0,
				"health_checks": {
					"checks": 2842133,
					"fails": 1,
					"unhealthy": 1,
					"last_passed": true
				},
				"downtime": 1012,
				"selected": "2022-10-17T20:38:35Z"
			}
		],
		"keepalive": 0,
		"zombies": 0,
		"zone": "bar.example.org-lxr-backend"
	}`

	validResponseUpstreamHGbackend = `{
		"peers": [
			{
				"id": 0,
				"server": "10.0.0.42:8084",
				"name": "10.0.0.42:8084",
				"backup": false,
				"weight": 1,
				"state": "up",
				"active": 1,
				"ssl": {
					"handshakes": 19803694,
					"handshakes_failed": 0,
					"session_reuses": 19803274
				},
				"requests": 19803806,
				"header_time": 10,
				"response_time": 10,
				"responses": {
					"1xx": 0,
					"2xx": 17210560,
					"3xx": 8364,
					"4xx": 1721,
					"5xx": 0,
					"codes": {
						"200": 17210560,
						"301": 7922,
						"304": 442,
						"400": 1,
						"404": 1086,
						"405": 634
					},
					"total": 17220645
				},
				"sent": 7525574329,
				"received": 16975650045,
				"fails": 2583043,
				"unavail": 0,
				"health_checks": {
					"checks": 2852546,
					"fails": 0,
					"unhealthy": 0,
					"last_passed": true
				},
				"downtime": 0,
				"selected": "2022-10-17T20:38:40Z"
			},
			{
				"id": 1,
				"server": "10.0.0.41:8084",
				"name": "10.0.0.41:8084",
				"backup": false,
				"weight": 1,
				"state": "down",
				"active": 0,
				"ssl": {
					"handshakes": 21808054,
					"handshakes_failed": 0,
					"session_reuses": 21807458
				},
				"requests": 21808225,
				"header_time": 11,
				"response_time": 11,
				"responses": {
					"1xx": 0,
					"2xx": 19223635,
					"3xx": 275,
					"4xx": 1089,
					"5xx": 0,
					"codes": {
						"200": 19223634,
						"206": 1,
						"301": 100,
						"304": 175,
						"404": 623,
						"405": 466
					},
					"total": 19224999
				},
				"sent": 10110237497,
				"received": 18490496811,
				"fails": 2583040,
				"unavail": 0,
				"health_checks": {
					"checks": 2842133,
					"fails": 1,
					"unhealthy": 1,
					"last_passed": true
				},
				"downtime": 1012,
				"selected": "2022-10-17T20:38:35Z"
			}
		],
		"keepalive": 0,
		"zombies": 0,
		"zone": "bar.example.org-hg-backend"
	}`

	validResponseGetUpstreamAllServersUp = `{
		"peers": [
			{
				"id": 0,
				"server": "10.0.0.42:8084",
				"name": "10.0.0.42:8084",
				"backup": false,
				"weight": 1,
				"state": "up",
				"active": 1,
				"ssl": {
					"handshakes": 19803694,
					"handshakes_failed": 0,
					"session_reuses": 19803274
				},
				"requests": 19803806,
				"header_time": 10,
				"response_time": 10,
				"responses": {
					"1xx": 0,
					"2xx": 17210560,
					"3xx": 8364,
					"4xx": 1721,
					"5xx": 0,
					"codes": {
						"200": 17210560,
						"301": 7922,
						"304": 442,
						"400": 1,
						"404": 1086,
						"405": 634
					},
					"total": 17220645
				},
				"sent": 7525574329,
				"received": 16975650045,
				"fails": 2583043,
				"unavail": 0,
				"health_checks": {
					"checks": 2852546,
					"fails": 0,
					"unhealthy": 0,
					"last_passed": true
				},
				"downtime": 0,
				"selected": "2022-10-17T20:38:40Z"
			},
			{
				"id": 1,
				"server": "10.0.0.41:8084",
				"name": "10.0.0.41:8084",
				"backup": false,
				"weight": 1,
				"state": "up",
				"active": 0,
				"ssl": {
					"handshakes": 21808054,
					"handshakes_failed": 0,
					"session_reuses": 21807458
				},
				"requests": 21808225,
				"header_time": 11,
				"response_time": 11,
				"responses": {
					"1xx": 0,
					"2xx": 19223635,
					"3xx": 275,
					"4xx": 1089,
					"5xx": 0,
					"codes": {
						"200": 19223634,
						"206": 1,
						"301": 100,
						"304": 175,
						"404": 623,
						"405": 466
					},
					"total": 19224999
				},
				"sent": 10110237497,
				"received": 18490496811,
				"fails": 2583040,
				"unavail": 0,
				"health_checks": {
					"checks": 2842133,
					"fails": 1,
					"unhealthy": 1,
					"last_passed": true
				},
				"downtime": 1012,
				"selected": "2022-10-17T20:38:35Z"
			}
		],
		"keepalive": 0,
		"zombies": 0,
		"zone": "demo-backend"
	}`

	validResponseGetUpstreamsZones = `{
		"demo-backend": {
			"zone": "foo.example.com-demo-backend"
		},
		"trac-backend": {
			"zone": "bar.example.com-trac-backend"
		},
		"hg-backend": {
			"zone": "bar.example.org-hg-backend"
		},
		"lxr-backend": {
			"zone": "bar.example.org-lxr-backend"
		}
	}`
)
