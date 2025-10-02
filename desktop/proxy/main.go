package main

import (
    "bytes"
    "flag"
    "fmt"
    "io"
    "net"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/miekg/dns"
    "github.com/sirupsen/logrus"
)

type DNSProxy struct {
    dohURL  string
    server  *dns.Server
    client  *http.Client
    cache   map[string]*dns.Msg
    log     *logrus.Logger
    metrics struct { queries, cacheHits, failures int }
}

func NewDNSProxy(host, port, dohURL string) *DNSProxy {
    log := logrus.New()
    log.SetLevel(logrus.InfoLevel)
    return &DNSProxy{
        dohURL: dohURL,
        server: &dns.Server{Addr: net.JoinHostPort(host, port), Net: "udp"},
        client: &http.Client{Timeout: 5 * time.Second},
        cache:  make(map[string]*dns.Msg),
        log:    log,
    }
}

func (p *DNSProxy) Start() error {
    dns.HandleFunc(".", p.handleDNS)
    p.log.Infof("Listening on %s", p.server.Addr)
    return p.server.ListenAndServe()
}

func (p *DNSProxy) handleDNS(w dns.ResponseWriter, r *dns.Msg) {
    p.metrics.queries++
    key := r.String()
    if resp, ok := p.cache[key]; ok {
        p.metrics.cacheHits++
        p.log.Debug("Cache hit")
        w.WriteMsg(resp)
        return
    }

    resp, err := p.queryDoH(r)
    if err != nil {
        p.metrics.failures++
        p.log.Errorf("DoH error: %v", err)
        msg := new(dns.Msg)
        msg.SetReply(r)
        msg.Rcode = dns.RcodeServerFailure
        w.WriteMsg(msg)
        return
    }

    ttl := 300 * time.Second
    if len(resp.Answer) > 0 {
        ttl = time.Duration(resp.Answer[0].Header().Ttl) * time.Second
    }
    p.cache[key] = resp
    go func() {
        time.Sleep(ttl)
        delete(p.cache, key)
    }()
    p.log.Infof("Resolved query: %s", r.Question[0].Name)
    w.WriteMsg(resp)
}

// func (p *DNSProxy) queryDoH(r *dns.Msg) (*dns.Msg, error) {
//     buf, err := r.Pack()
//     if err != nil {
//         return nil, err
//     }
//     req, err := http.NewRequest("POST", p.dohURL, bytes.NewReader(buf))
//     if err != nil {
//         return nil, err
//     }
//     req.Header.Set("Content-Type", "application/dns-message")
//     req.Header.Set("Accept", "application/dns-message")

//     resp, err := p.client.Do(req)
//     if err != nil {
//         return nil, err
//     }
//     defer resp.Body.Close()

//     if resp.StatusCode != http.StatusOK {
//         return nil, fmt.Errorf("DoH request failed: %s", resp.Status)
//     }

//     body, err := io.ReadAll(resp.Body)
//     if err != nil {
//         return nil, err
//     }

//     msg := &dns.Msg{}
//     if err := msg.Unpack(body); err != nil {
//         return nil, err
//     }
//     return msg, nil
// }
func (p *DNSProxy) queryDoH(r *dns.Msg) (*dns.Msg, error) {
    buf, err := r.Pack()
    if err != nil {
        p.log.Errorf("Failed to pack DNS message: %v", err)
        return nil, err
    }
    req, err := http.NewRequest("POST", p.dohURL, bytes.NewReader(buf))
    if err != nil {
        p.log.Errorf("Failed to create HTTP request: %v", err)
        return nil, err
    }
    req.Header.Set("Content-Type", "application/dns-message")
    req.Header.Set("Accept", "application/dns-message")

    resp, err := p.client.Do(req)
    if err != nil {
        p.log.Errorf("HTTP request failed: %v", err)
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        p.log.Errorf("DoH request failed: %s", resp.Status)
        return nil, fmt.Errorf("DoH request failed: %s", resp.Status)
    }

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        p.log.Errorf("Failed to read response body: %v", err)
        return nil, err
    }

    msg := &dns.Msg{}
    if err := msg.Unpack(body); err != nil {
        p.log.Errorf("Failed to unpack DNS response: %v", err)
        return nil, err
    }
    return msg, nil
}

func (p *DNSProxy) Shutdown() {
    p.log.Infof("Shutting down. Metrics: queries=%d, cacheHits=%d, failures=%d", p.metrics.queries, p.metrics.cacheHits, p.metrics.failures)
    p.server.Shutdown()
}

func main() {
    // host := flag.String("host", "127.0.0.1", "Listen host")
    // port := flag.String("port", "5353", "Listen port")
    // dohURL := flag.String("doh", "https://cloudflare-dns.com/dns-query", "DoH server URL")
    // flag.Parse()
    host := flag.String("host", "127.0.0.1", "Listen host")
    port := flag.String("port", "5353", "Listen port")
    dohURL := flag.String("doh", "https://dns.google/dns-query", "DoH server URL")
    flag.Parse()

    proxy := NewDNSProxy(*host, *port, *dohURL)
    sigs := make(chan os.Signal, 1)
    signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        <-sigs
        proxy.Shutdown()
    }()

    if err := proxy.Start(); err != nil {
        proxy.log.Fatalf("Failed to start: %v", err)
    }
}