// Package broker implements the capability broker: the only entity that can
// grant an action. Deny by default — a capability must be explicitly granted
// in the frozen manifest or the action does not exist.
package broker

import (
	"encoding/json"
	"fmt"
	"net"
	"path"
	"strings"

	"github.com/desmondkam/openselo/internal/manifest"
)

type CapabilityRequest struct {
	RunID  string   `json:"run_id"`
	Kind   string   `json:"kind"` // filesystem.read | filesystem.write | exec | git.commit | git.push
	Target string   `json:"target"`
	Args   []string `json:"args,omitempty"`
}

type Decision struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
	Policy  string `json:"policy"`
}

// Broker evaluates requests against a frozen manifest.
type Broker struct {
	m        *manifest.RunManifest
	OnDecide func(req CapabilityRequest, d Decision) // optional evidence hook
}

func New(m *manifest.RunManifest) *Broker {
	return &Broker{m: m}
}

// Authorize is the single entry point. Any unknown kind is denied.
func (b *Broker) Authorize(req CapabilityRequest) Decision {
	var d Decision
	if req.RunID != b.m.RunID {
		d = deny("run_id_mismatch", "run_id")
	} else {
		switch req.Kind {
		case "filesystem.write":
			d = b.fsWrite(req)
		case "filesystem.read":
			d = b.fsRead(req)
		case "exec":
			d = b.exec(req)
		case "git.commit":
			d = boolCap(b.m.Capability.Git.Commit, "git.commit")
		case "git.push":
			d = boolCap(b.m.Capability.Git.Push, "git.push")
		default:
			d = deny("unknown_kind", req.Kind)
		}
	}
	if b.OnDecide != nil {
		b.OnDecide(req, d)
	}
	return d
}

func (b *Broker) fsWrite(req CapabilityRequest) Decision {
	if len(b.m.Capability.Filesystem.Write) == 0 {
		return deny("no_write_capability", "filesystem.write")
	}
	if !matchesAny(req.Target, b.m.Capability.Filesystem.Write) {
		return deny("path_outside_scope", "filesystem.write")
	}
	return Decision{Allowed: true, Reason: "ok", Policy: "filesystem.write"}
}

func (b *Broker) fsRead(req CapabilityRequest) Decision {
	if len(b.m.Capability.Filesystem.Read) == 0 {
		return deny("no_read_capability", "filesystem.read")
	}
	if !matchesAny(req.Target, b.m.Capability.Filesystem.Read) {
		return deny("path_outside_scope", "filesystem.read")
	}
	return Decision{Allowed: true, Reason: "ok", Policy: "filesystem.read"}
}

func (b *Broker) exec(req CapabilityRequest) Decision {
	if len(b.m.Capability.Exec.Allow) == 0 {
		return deny("no_exec_capability", "exec")
	}
	cmd := req.Target
	if len(req.Args) > 0 {
		cmd = strings.TrimSpace(strings.Join(append([]string{req.Target}, req.Args...), " "))
	}
	if !matchesAny(cmd, b.m.Capability.Exec.Allow) {
		return deny("command_not_allowed", "exec")
	}
	return Decision{Allowed: true, Reason: "ok", Policy: "exec"}
}

func boolCap(granted bool, policy string) Decision {
	if granted {
		return Decision{Allowed: true, Reason: "ok", Policy: policy}
	}
	return deny("capability_not_granted", policy)
}

func deny(reason, policy string) Decision {
	return Decision{Allowed: false, Reason: reason, Policy: policy}
}

// matchesAny matches a target against glob patterns using path.Clean
// semantics, so "./src/a.ts" and "src/a.ts" are the same path.
func matchesAny(target string, patterns []string) bool {
	clean := path.Clean(target)
	for _, p := range patterns {
		p = strings.TrimPrefix(path.Clean(p), "./")
		if matched, err := path.Match(p, clean); err == nil && matched {
			return true
		}
		// allow prefix directory grant: "src/" covers "src/a/b.ts"
		if strings.HasSuffix(p, "/**") || strings.HasSuffix(p, "/") {
			dir := strings.TrimSuffix(p, "**")
			dir = strings.TrimSuffix(dir, "/")
			if dir == "" || strings.HasPrefix(clean, dir+"/") {
				return true
			}
		}
	}
	return false
}

// Serve runs a JSON-lines broker over a unix socket: each connection reads
// one request, writes one decision, closes.
func (b *Broker) Serve(sockPath string) error {
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", sockPath, err)
	}
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go b.handle(conn)
	}
}

func (b *Broker) handle(conn net.Conn) {
	defer conn.Close()
	var req CapabilityRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		// unreadable request = deny
		_ = json.NewEncoder(conn).Encode(deny("malformed_request", "rpc"))
		return
	}
	_ = json.NewEncoder(conn).Encode(b.Authorize(req))
}