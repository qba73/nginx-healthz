package nginxhealthz

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

type responseUpstreams struct {
	Upstreams []struct {
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
	} `json:"upstreams"`
}

type UpstreamStatus struct {
	Total int
	Up    int
	Down  int
}

type ServerStatus struct {
	Server string
	Name   string
	Status string
}

type Client struct {
	Version    int
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		Version: 8,
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *Client) GetStats() (UpstreamStatus, error) {
	url := fmt.Sprintf("%s/api/%d/http", c.BaseURL, c.Version)
	var res responseUpstream
	if err := c.get(url, &res); err != nil {
		return UpstreamStatus{}, err
	}
	if len(res.Peers) < 1 {
		return UpstreamStatus{}, errors.New("no servers in upstream")
	}

	servers := make([]ServerStatus, len(res.Peers))
	for i, p := range res.Peers {
		s := ServerStatus{
			Server: p.Server,
			Name:   p.Name,
			Status: p.State,
		}
		servers[i] = s
	}

	total := len(servers)
	var up int
	for _, i := range servers {
		if i.Status == "up" {
			up++
		}
	}
	down := total - up

	return UpstreamStatus{
		Total: total,
		Up:    up,
		Down:  down,
	}, nil
}

func (c *Client) GetZones() ([]string, error) {
	url := fmt.Sprintf("%s/api/%d/http/upstreams?fields=zone", c.BaseURL, c.Version)

	var res interface{}

	err := c.get(url, &res)
	if err != nil {
		return nil, fmt.Errorf("retrieving zones: %w", err)
	}

	var zones []string

	m := res.(map[string]interface{})
	for _, v := range m {
		switch vv := v.(type) {
		case map[string]interface{}:
			for _, z := range vv {
				zones = append(zones, z.(string))
			}
		}
	}

	return zones, nil
}

func (c *Client) get(url string, data interface{}) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTPClient.Do(req)
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
