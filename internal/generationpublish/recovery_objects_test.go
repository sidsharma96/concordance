package generationpublish

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
)

type recoveryTraffic struct {
	GET, HEAD, PUT    int
	Request, Response int64
}

func (a recoveryTraffic) minus(b recoveryTraffic) recoveryTraffic {
	return recoveryTraffic{a.GET - b.GET, a.HEAD - b.HEAD, a.PUT - b.PUT, a.Request - b.Request, a.Response - b.Response}
}

// The underlying existing remote fixture checks conditional writes, validators,
// and bounds each body to 1 MiB. This wrapper caps distinct keys and records actual
// peer-side body reads/writes, not headers, TCP bytes or delivery to the SDK.
// The owner samples only at worker barriers, with no next command dispatched.
type recoveryObjects struct {
	next       http.Handler
	remote     *remote
	mu         sync.Mutex
	work       sync.WaitGroup
	traffic    recoveryTraffic
	drop, lost bool
}
type recoveryBody struct {
	io.ReadCloser
	owner *recoveryObjects
}

func (b recoveryBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.owner.mu.Lock()
	b.owner.traffic.Request += int64(n)
	b.owner.mu.Unlock()
	return n, err
}
func (p *recoveryObjects) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.work.Add(1)
	defer p.work.Done()
	p.mu.Lock()
	switch r.Method {
	case "GET":
		p.traffic.GET++
	case "HEAD":
		p.traffic.HEAD++
	case "PUT":
		p.traffic.PUT++
	}
	p.mu.Unlock()
	r.Body = recoveryBody{r.Body, p}
	defer r.Body.Close()
	if r.Method == "PUT" && p.remote.size() >= 32 {
		w.WriteHeader(507)
		return
	}
	response := httptest.NewRecorder()
	p.next.ServeHTTP(response, r)
	p.mu.Lock()
	drop := p.drop && !p.lost && r.Method == "PUT" && response.Code == http.StatusOK
	if drop {
		p.drop = false
	}
	p.mu.Unlock()
	if drop {
		// Bytes are already stored. Closing the real socket makes the SDK observe
		// a missing reply; the production adapter's verified GET must reconcile it.
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			err = conn.Close()
		}
		p.mu.Lock()
		p.lost = err == nil
		p.mu.Unlock()
		return
	}
	for k, v := range response.Header() {
		w.Header()[k] = v
	}
	w.WriteHeader(response.Code)
	n, _ := w.Write(response.Body.Bytes())
	p.mu.Lock()
	p.traffic.Response += int64(n)
	p.mu.Unlock()
}
func (p *recoveryObjects) snapshot() recoveryTraffic {
	p.work.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.traffic
}
