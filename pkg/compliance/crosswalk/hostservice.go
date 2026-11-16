// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package crosswalk

import (
	"regexp"

	"unheaded/pkg/ports"
)

// KindHostService is ref "<host>:<service>", service from the fixed table
// HostServices (ADR-097, amendment of 2026-11-15; FND-037). It reads what
// runs: the record passes when the service is running on that host, its
// auth setting is on, and a request with no credentials to its protected
// route answers 401. The catalog can name a service, never a container, a
// port, a path or a command.
const KindHostService = "host-service"

var (
	hostServiceRefRe  = regexp.MustCompile(`^([a-z0-9][a-z0-9-]{0,62}):([a-z0-9][a-z0-9-]{0,62})$`)
	hostServiceNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

// HostService is one first-party HTTP service as the collector reads it.
type HostService struct {
	// Port the service listens on (pkg/ports).
	Port int
	// Method and Route: one request the service refuses without
	// credentials. A GET of a protected read where the service has one;
	// Why says what else it is and why it changes nothing.
	Method string
	Route  string
	Why    string
	// AuthVar, when set, is the environment variable holding the bearer
	// token the service checks itself (check-service-auth.sh's TOKEN_AUTH);
	// it must be set and not empty. Empty: the service uses pkg/auth, and
	// AUTH_ENABLED must be exactly "true".
	AuthVar string
	// Exempt, when set, is why the service has nothing to read: its routes
	// are public by design (check-service-auth.sh's EXEMPT). An exempt
	// service has no ref.
	Exempt string
	// Runs says how the service runs on each host that runs it.
	Runs map[string]ServiceRun
}

// ServiceRun is how a service runs on one host: a compose container, or a
// systemd unit from a package.
type ServiceRun struct {
	Container string // compose container_name
	Unit      string // systemd unit, without ".service"
	Package   string // the unit's package (its version goes in the detail)
	// Addr is where the collector reaches the port. Empty: the container's
	// own address on the compose network, for a service that publishes no
	// port.
	Addr string
	// TLSName is the name the certificate is checked against when Addr is
	// not one of its names.
	TLSName string
	// CA is the CA file, on the collector's host, that issued the
	// service's certificate.
	CA string
}

const hostCA = "/etc/unheaded/tls/ca.pem"

func onWest(container string) map[string]ServiceRun {
	return map[string]ServiceRun{"west": {Container: container, Addr: "127.0.0.1", CA: hostCA}}
}

// HostServices is every first-party HTTP service: the compose services the
// gate scripts/compliance/check-service-auth.sh counts (a test holds the
// two sets equal). East's entries are added when its redeploy is done
// (S-E1-LIVE Step 218); until then UH-AUTHN-01 keeps `declared:`.
var HostServices = map[string]HostService{
	"wotan":    {Port: ports.WotanHTTP, Method: "GET", Route: "/api/v1/admin/pending", Runs: onWest("unheaded-wotan")},
	"timeguru": {Port: ports.Timeguru, Method: "GET", Route: "/api/v1/timeline", Runs: onWest("unheaded-timeguru")},
	// Compose profile "lxd": not run on west until a tenant is placed
	// (ADR-147 decisions 7 and 14), so it has no ref, as sword.
	"cuirass": {Port: ports.Cuirass, Method: "GET", Route: "/api/v1/containers"},
	"akira": {Port: ports.Akira, Method: "GET", Route: "/",
		Why:  "Akira's one API route is an open read (cmd/akira apiHandler); every other path is refused before the mux",
		Runs: onWest("unheaded-akira")},
	"dashboard-backend": {Port: ports.DashboardBackend, Method: "GET", Route: "/api/v1/logs", Runs: onWest("unheaded-dashboard")},
	"kanban-app": {Port: ports.KanbanApp, Method: "DELETE", Route: "/api/v1/tasks/",
		Why:  "the board opens every read by design (cmd/kanban-app auth_bff.go); a DELETE with no task id is 401 with auth on and a 400 from the handler with auth off, and deletes nothing either way",
		Runs: onWest("unheaded-kanban")},
	"logstore-writer": {Port: ports.LogstoreWriter, Method: "POST", Route: "/ingest", AuthVar: "LOGSTORE_INGEST_TOKEN",
		Why:  "its one route is a POST; the bearer token is checked before the body is read (cmd/logstore-writer ingest.go), and the probe's body is empty",
		Runs: map[string]ServiceRun{"west": {Container: "unheaded-logstore-writer", TLSName: "logstore-writer", CA: hostCA}}},
	// Compose profile "deploy": started for a deploy window, so no host
	// runs it as a standing service and it has no ref.
	"sword": {Port: ports.Sword, Method: "GET", Route: "/api/v1/deployments"},
	"auth":  {Exempt: "OIDC issuer: login, JWKS and discovery are public by design (ADR-103 §3)"},
}

// checkHostServiceRef says why ref cannot be a host-service source, or "".
func checkHostServiceRef(ref string) string {
	m := hostServiceRefRe.FindStringSubmatch(ref)
	if m == nil {
		return "is not <host>:<service>"
	}
	svc, ok := HostServices[m[2]]
	switch {
	case !ok:
		return "names " + m[2] + ", which is not a service in the table"
	case svc.Exempt != "":
		return "names " + m[2] + ", which is exempt (" + svc.Exempt + ")"
	}
	if _, ok := svc.Runs[m[1]]; !ok {
		return "names " + m[2] + ", which does not run on " + m[1] + " in the table"
	}
	return ""
}

// SplitHostServiceRef returns the host and service of a valid ref.
func SplitHostServiceRef(ref string) (host, service string, ok bool) {
	if checkHostServiceRef(ref) != "" {
		return "", "", false
	}
	m := hostServiceRefRe.FindStringSubmatch(ref)
	return m[1], m[2], true
}
