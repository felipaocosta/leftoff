package fetch

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
)

// ErrNoBrowser means the local browser solver found no Chromium-family
// browser (Chrome, Chromium, Edge or Brave) on this computer.
var ErrNoBrowser = errors.New("no Chrome, Chromium, Edge or Brave found (install one or set BROWSER_BIN)")

// Solver gets a page past an anti-bot challenge (Cloudflare "Just a
// moment…") and returns the clearance that lets plain requests to the same
// host through for a while (RF-56). Implementations: FlareSolverr here and a
// local browser in fetch/browser.
type Solver interface {
	Name() string
	Solve(ctx context.Context, req *Request) (*Solution, error)
}

// Solution is a page fetched by a Solver plus the clearance to reuse.
type Solution struct {
	Response  *Response
	Clearance Clearance
}

// Clearance holds the cookies and the User-Agent that passed a challenge.
// Cloudflare ties its cookies to the User-Agent, so both are replayed.
type Clearance struct {
	Cookies   []*http.Cookie
	UserAgent string
}

func (c Clearance) empty() bool { return len(c.Cookies) == 0 && c.UserAgent == "" }

// clearances keeps the latest Clearance per host, in memory only. Cookies
// are never logged.
type clearances struct {
	mu     sync.Mutex
	byHost map[string]Clearance
}

func newClearances() *clearances { return &clearances{byHost: map[string]Clearance{}} }

func (c *clearances) set(host string, cl Clearance) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byHost[strings.ToLower(host)] = cl
}

// get returns the clearance of host or of a parent domain (cookies set for
// .site.org also apply to www.site.org).
func (c *clearances) get(host string) (Clearance, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	h := strings.ToLower(host)
	for {
		if cl, ok := c.byHost[h]; ok {
			return cl, true
		}
		dot := strings.IndexByte(h, '.')
		if dot < 0 || !strings.Contains(h[dot+1:], ".") {
			return Clearance{}, false
		}
		h = h[dot+1:]
	}
}

// apply adds the clearance cookies and User-Agent to an outgoing request.
func (cl Clearance) apply(r *http.Request) {
	if cl.UserAgent != "" {
		r.Header.Set("User-Agent", cl.UserAgent)
	}
	for _, ck := range cl.Cookies {
		r.AddCookie(&http.Cookie{Name: ck.Name, Value: ck.Value})
	}
}
