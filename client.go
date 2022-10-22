package nginxhealthz

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type responseUpstream struct {
	Peers []struct {
		ID     int    `json:"id"`
		Server string `json:"server"`
		Name   string `json:"name"`
		Backup bool   `json:"backup"`
		Weight int    `json:"weight"`
		State  string `json:"state"`
		Active int    `json:"active"`
		Ssl    struct {
			Handshakes       int `json:"handshakes"`
			HandshakesFailed int `json:"handshakes_failed"`
			SessionReuses    int `json:"session_reuses"`
		} `json:"ssl"`
		Requests     int `json:"requests"`
		HeaderTime   int `json:"header_time"`
		ResponseTime int `json:"response_time"`
		Responses    struct {
			OneXx   int `json:"1xx"`
			TwoXx   int `json:"2xx"`
			ThreeXx int `json:"3xx"`
			FourXx  int `json:"4xx"`
			FiveXx  int `json:"5xx"`
			Codes   struct {
				Num200 int `json:"200"`
				Num301 int `json:"301"`
				Num304 int `json:"304"`
				Num400 int `json:"400"`
				Num404 int `json:"404"`
				Num405 int `json:"405"`
			} `json:"codes"`
			Total int `json:"total"`
		} `json:"responses"`
		Sent         int64 `json:"sent"`
		Received     int64 `json:"received"`
		Fails        int   `json:"fails"`
		Unavail      int   `json:"unavail"`
		HealthChecks struct {
			Checks     int  `json:"checks"`
			Fails      int  `json:"fails"`
			Unhealthy  int  `json:"unhealthy"`
			LastPassed bool `json:"last_passed"`
		} `json:"health_checks"`
		Downtime int       `json:"downtime"`
		Selected time.Time `json:"selected"`
	} `json:"peers"`
	Keepalive int    `json:"keepalive"`
	Zombies   int    `json:"zombies"`
	Zone      string `json:"zone"`
}

type Stats struct {
	Total int
	Up    int
	Down  int
}

type option func(*Client) error

func WithHTTPClient(h *http.Client) option {
	return func(c *Client) error {
		if h == nil {
			return errors.New("nil http client")
		}
		c.httpClient = h
		return nil
	}
}

func WithVersion(v int) option {
	return func(c *Client) error {
		switch v {
		case 1, 2, 3, 4, 5, 6, 7, 8:
			c.version = v
			return nil
		default:
			return fmt.Errorf("unsupported NGINX version: %d", v)
		}
	}
}

type Client struct {
	version    int
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, opts ...option) (*Client, error) {
	_, err := url.ParseRequestURI(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %s, %w", baseURL, err)
	}

	c := Client{
		version: 8,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	for _, opt := range opts {
		if err := opt(&c); err != nil {
			return nil, err
		}
	}
	return &c, nil
}

func (c *Client) GetStatsFor(upstream string) (Stats, error) {
	url := fmt.Sprintf("%s/api/%d/http/upstreams/%s", c.baseURL, c.version, upstream)
	var res responseUpstream
	if err := c.get(url, &res); err != nil {
		return Stats{}, err
	}
	return calculateStatsFor(upstream, res)
}

func calculateStatsFor(upstream string, res responseUpstream) (Stats, error) {
	if len(res.Peers) < 1 {
		return Stats{}, errors.New("no servers in upstream")
	}

	total := len(res.Peers)
	up := 0

	for _, p := range res.Peers {
		if p.State == "up" {
			up++
		}
	}
	down := total - up
	return Stats{Total: total, Up: up, Down: down}, nil
}

func (c *Client) GetUpstreamsFor(hostname string) (map[string][]string, error) {
	url := fmt.Sprintf("%s/api/%d/http/upstreams?fields=zone", c.baseURL, c.version)

	var response interface{}
	err := c.get(url, &response)
	if err != nil {
		return nil, fmt.Errorf("retrieving zones: %w", err)
	}
	return hostnameUpstreamsFromResponse(hostname, response), nil
}

func hostnameUpstreamsFromResponse(hostname string, res interface{}) map[string][]string {
	hostUpstreams := make(map[string][]string)
	m := res.(map[string]interface{})

	for u, v := range m {
		switch vv := v.(type) {
		case map[string]interface{}:
			for _, z := range vv {
				host := z.(string)
				host = strings.Split(host, "-")[0]
				if host != hostname {
					continue
				}
				_, ok := hostUpstreams[host]
				if !ok {
					hostUpstreams[host] = []string{u}
					continue
				}
				hostUpstreams[host] = append(hostUpstreams[host], u)
			}
		}
	}
	return hostUpstreams
}

func (c *Client) GetStatsForHost(hostname string) (Stats, error) {
	upstreams, err := c.GetUpstreamsFor(hostname)
	if err != nil {
		return Stats{}, fmt.Errorf("getting stats for host %s: %w", hostname, err)
	}
	ux, ok := upstreams[hostname]
	if !ok {
		return Stats{}, fmt.Errorf("no stat data for host %s", hostname)
	}
	return c.GetStatsForUpstreams(ux), nil
}

func (c *Client) GetStatsForUpstreams(ux []string) Stats {
	var total, up, down uint64

	var wg sync.WaitGroup
	wg.Add(len(ux))

	for _, u := range ux {
		go func(upstream string) {
			defer wg.Done()
			stat, err := c.GetStatsFor(upstream)
			if err != nil {
				return
			}
			atomic.AddUint64(&total, uint64(stat.Total))
			atomic.AddUint64(&up, uint64(stat.Up))
			atomic.AddUint64(&down, uint64(stat.Down))
		}(u)
	}
	wg.Wait()
	return Stats{Total: int(total), Up: int(up), Down: int(down)}
}

func (c *Client) get(url string, data interface{}) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("got response code: %v", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if err := json.Unmarshal(body, data); err != nil {
		return fmt.Errorf("unmarshaling response body: %w", err)
	}
	return nil
}
