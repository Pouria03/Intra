# Desktop DoH Client (Windows/Linux) — Proposal

**Purpose**: Build a cross-platform CLI DNS-to-DoH proxy replicating Intra’s Android functionality for bypassing DNS-based censorship.

**Scope (v1)**:
- CLI proxy listens on 127.0.0.1:5353 (UDP), forwards to DoH (e.g., Cloudflare).
- Features: Caching, timeouts, retries, logging, metrics.
- Uses Intra’s Go code (`github.com/Jigsaw-Code/Intra/go/intra`).
- Manual DNS setup instructions; future auto-setup.
- Go binaries for Windows/Linux.

**Implementation**:
- Language: Go
- Libraries: `miekg/dns`, `sirupsen/logrus`
- License: Apache-2.0

**Next Steps**:
- Implement proxy in `desktop/proxy/main.go`.
- Test censorship bypass on Windows/Linux.
