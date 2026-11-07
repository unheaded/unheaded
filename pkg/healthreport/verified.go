// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package healthreport

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"unheaded/pkg/health"
)

// MaxNodeBytes caps a host or node name in a report.
const MaxNodeBytes = 64

// akiraIdentity is the one identity that votes on services other than its
// own: checking them is what Akira does.
const akiraIdentity = "akira"

// Why Admit refuses a report.
var (
	ErrBadReport     = errors.New("health report malformed")
	ErrUnverified    = errors.New("health report has no verified sender")
	ErrNotOwnService = errors.New("health report is on a service its sender does not own")
	ErrBadNode       = errors.New("health report names no usable node")
)

// nodeName is what a host name looks like; "@" is the reporter separator.
var nodeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ballotAliases are ballot names a deployment gives an identity through
// $HEALTH_REPORT_SERVICE, beyond ballotNames.
var ballotAliases = map[string][]string{
	"unheaded-daemon": {"cuirass"},
}

// ownsService reports whether identity reports under this ballot name.
func ownsService(identity, service string) bool {
	if service == identity || service == ballotNames[identity] {
		return true
	}
	for _, alias := range ballotAliases[identity] {
		if service == alias {
			return true
		}
	}
	return false
}

func validNode(s string) bool {
	return len(s) <= MaxNodeBytes && nodeName.MatchString(s)
}

// Admit decides what a message on Topic may count as. sender is the
// message's verified sender, the subject Wotan took from the publisher's
// token; a consumer passes requireVerified when it runs with auth on.
//
// With a sender, the report's reporter is never the one the payload gives:
// a service votes only on its own ballot name, as "<service>@<host>", so an
// identity holds a vote on no service but its own. Akira votes on any
// service, as its node. The host is still the publisher's word, so a sender
// can name as many hosts as it likes: each consumer bounds what it keeps of
// them (Akira's ballot; the dashboard's Well writer, per run of the process).
//
// Without a sender the report is refused when requireVerified is set. Auth
// off has nothing to check it against, so it passes as written.
func Admit(sender string, requireVerified bool, payload []byte) (health.HealthReport, error) {
	var r health.HealthReport
	if err := json.Unmarshal(payload, &r); err != nil {
		return r, fmt.Errorf("%w: %v", ErrBadReport, err)
	}
	if r.Service == "" {
		return r, fmt.Errorf("%w: no service", ErrBadReport)
	}
	switch {
	case sender == "":
		if requireVerified {
			return r, ErrUnverified
		}
	case sender == akiraIdentity:
		if !validNode(r.Reporter) {
			return r, ErrBadNode
		}
	default:
		if !ownsService(sender, r.Service) {
			return r, ErrNotOwnService
		}
		if !validNode(r.Host) {
			return r, ErrBadNode
		}
		r.Reporter = r.Service + "@" + r.Host
	}
	return r, nil
}
