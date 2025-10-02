package main

import (
    "context"
    "flag"
    "net"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/Jigsaw-Code/Intra/go/intra"
    "github.com/miekg/dns"
    "github.com/sirupsen/logrus"
)

type DNSProxy struct {
    dohURL  string
    server  *dns.Server
    client  *intra.DoHClient
    cache   *intra.SimpleCache
    log     *logrus.Logger
    metrics struct {
        queries, cacheHits, failures int
    }
}

func NewDNSProxy(host, port, dohURL string) *DNSProxy {
    log := logrus.New()
    log.SetLevel(logrus.InfoLevel)
    return &DNSProxy{
        dohURL: dohURL,
        server: &dns.Server{Addr: net.JoinHostPort(host, port), Net: "udp"},
        client: intra.NewDoHClient(dohURL),
        cache:  intra.NewSimpleCache(100),
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
    if resp, ok := p.cache.Get(key); ok {
        p.metrics.cacheHits++
        p.log.Debug("Cache hit")
        w.WriteMsg(resp)
        return
    }

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    resp, err := p.client.Query(ctx, r)
    if err != nil {
        p.metrics.failures++
        p.log.Errorf("DoH error: %v", err)
        w.WriteMsg(&dns.Msg{Response: true, Rcode: dns.RcodeServerFailure})
        return
    }

    ttl := time.Duration(resp.Answer[0].Header().Ttl) * time.Second
    p.cache.Set(key, resp, ttl)
    p.log.Infof("Resolved query: %s", r.Question[0].Name)
    w.WriteMsg(resp)
}

func (p *DNSProxy) Shutdown() {
    p.log.Infof("Shutting down. Metrics: queries=%d, cacheHits=%d, failures=%d",
        p.metrics.queries, p.metrics.cacheHits, p.metrics.failures)
    p.server.Shutdown()
}

func main() {
    host := flag.String("host", "127.0.0.1", "Listen host")
    port := flag.String("port", "5353", "Listen port")
    dohURL := flag.String("doh", "https://cloudflare-dns.com/dns-query", "DoH server URL")
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
