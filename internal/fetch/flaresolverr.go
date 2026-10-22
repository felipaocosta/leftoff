package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FlareSolverr solves challenges through a FlareSolverr service
// (FLARESOLVERR_URL, e.g. http://localhost:8191), which runs a headless
// browser. Its HTTP client is separate from the site rate limit: the
// service is local and the Client already serializes requests per host.
type FlareSolverr struct {
	endpoint   string
	maxTimeout time.Duration
	maxBody    int64
	http       *http.Client
}

// NewFlareSolverr returns a solver for the service at baseURL.
func NewFlareSolverr(baseURL string) *FlareSolverr {
	const maxTimeout = 60 * time.Second
	return &FlareSolverr{
		endpoint:   strings.TrimSuffix(baseURL, "/") + "/v1",
		maxTimeout: maxTimeout,
		maxBody:    5 << 20,
		http:       &http.Client{Timeout: maxTimeout + 15*time.Second},
	}
}

// Name implements Solver.
func (*FlareSolverr) Name() string { return "flaresolverr" }

type fsRequest struct {
	Cmd        string `json:"cmd"`
	URL        string `json:"url"`
	MaxTimeout int64  `json:"maxTimeout"`
	PostData   string `json:"postData,omitempty"`
}

type fsResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Solution struct {
		URL       string `json:"url"`
		Status    int    `json:"status"`
		Response  string `json:"response"`
		UserAgent string `json:"userAgent"`
		Cookies   []struct {
			Name   string  `json:"name"`
			Value  string  `json:"value"`
			Domain string  `json:"domain"`
			Path   string  `json:"path"`
			Expiry float64 `json:"expiry"`
		} `json:"cookies"`
	} `json:"solution"`
}

// Solve implements Solver. POST requests send the body as form data;
// FlareSolverr does not forward custom headers.
func (f *FlareSolverr) Solve(ctx context.Context, req *Request) (*Solution, error) {
	body := fsRequest{Cmd: "request.get", URL: req.URL, MaxTimeout: f.maxTimeout.Milliseconds()}
	if req.Method == http.MethodPost {
		body.Cmd, body.PostData = "request.post", string(req.Body)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("flaresolverr: encode: %w", err)
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, f.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("flaresolverr: %w", err)
	}
	hreq.Header.Set("Content-Type", "application/json")
	hresp, err := f.http.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("flaresolverr: %w", err)
	}
	defer func() { _ = hresp.Body.Close() }() // fully read below
	raw, err := io.ReadAll(io.LimitReader(hresp.Body, 2*f.maxBody))
	if err != nil {
		return nil, fmt.Errorf("flaresolverr: read: %w", err)
	}
	var out fsResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("flaresolverr: decode (HTTP %d): %w", hresp.StatusCode, err)
	}
	if out.Status != "ok" {
		return nil, fmt.Errorf("flaresolverr: %s", out.Message)
	}
	return f.solution(req, &out)
}

func (f *FlareSolverr) solution(req *Request, out *fsResponse) (*Solution, error) {
	sol := out.Solution
	if int64(len(sol.Response)) > f.maxBody {
		return nil, fmt.Errorf("flaresolverr %s: %w", req.URL, ErrTooLarge)
	}
	final, err := url.Parse(firstNonEmpty(sol.URL, req.URL))
	if err != nil {
		return nil, fmt.Errorf("flaresolverr: bad url %q: %w", sol.URL, err)
	}
	cl := Clearance{UserAgent: sol.UserAgent}
	for _, c := range sol.Cookies {
		ck := &http.Cookie{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path}
		if c.Expiry > 0 {
			ck.Expires = time.Unix(int64(c.Expiry), 0).UTC()
		}
		cl.Cookies = append(cl.Cookies, ck)
	}
	status := sol.Status
	if status == 0 {
		status = http.StatusOK
	}
	return &Solution{
		Response:  &Response{URL: final, StatusCode: status, Header: http.Header{}, Body: []byte(sol.Response)},
		Clearance: cl,
	}, nil
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
