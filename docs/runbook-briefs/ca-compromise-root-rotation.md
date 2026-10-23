<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Brief: ca-compromise-root-rotation (runbooks/pki/ca-compromise-root-rotation.yaml)

**Scenario.** A Kingdom CA private key is suspected exposed (copied, left
world-readable, found in a log, a backup or git). Replace the root, reissue
everything it signed, and stop trusting the old root, without an outage
longer than one service restart at a time.

**Owner** blackmage. **Risk** critical. **requires_sudo** true. Duration about 2h.

**Two CAs exist (say which one is compromised first):**
- Service mesh CA: `/var/lib/unheaded/pki/ca/` on west and east, service
  certs in `/var/lib/unheaded/pki/services/<svc>.{crt,key}`; made by
  `runbooks/pki/generate-kingdom-certs.yaml` and `go run ./cmd/cert-gen -out <dir>`
  (writes `ca.pem`, `ca-key.pem`, `<service>.pem`, `<service>-key.pem`);
  rotated per service by `runbooks/pki/rotate-service-cert.yaml`; gateway
  TLS by `runbooks/security/tls-cert-rotation.yaml`.
- The Well's CA: `/etc/unheaded/well-tls/` (`ca.crt`, `ca.key` root 0600,
  `server.crt`, `server.key`) made by `sudo scripts/well/make-tls.sh <dir>`;
  it refuses to overwrite an existing CA, so a new root goes into a NEW
  directory, then `WELL_TLS_DIR` points compose at it.

**Known open items (state them in notes; do not fix here):** compose mounts
the whole Well TLS directory, `ca.key` included, into the postgres container
(`${WELL_TLS_DIR}:/etc/postgresql/tls:ro`); the existing PKI runbooks chown
keys to the group `unheaded`, which no longer exists (every service has its
own account, `pkg/uids`): this runbook uses each service's own group.

**Order:**
1. Contain (manual, irreversible: true): stop issuing; move the old CA key
   offline (`sudo mv` to removable media); record its fingerprint
   (`openssl x509 -noout -fingerprint -sha256 -in <old ca cert>`) in the
   incident notes (`runbooks/security/incident-response.yaml`).
2. New root (manual, key handling): `go run ./cmd/cert-gen -out "$NEW_PKI"`
   or `make-tls.sh` into a new dir. Parameter `NEW_PKI` default
   `/var/lib/unheaded/pki-new` (root 0700).
3. Reissue every service certificate and the Well server certificate from
   the new root.
4. Trust bundle with old + new roots during an overlap window:
   `cat "$NEW_PKI/ca.pem" <old ca> > "$NEW_PKI/bundle.pem"`; deploy the
   bundle as the clients' CA first.
5. Restart services one at a time, each verified before the next, in the
   ADR-103 rollout order (Cuirass first, Wotan last).
6. Verify each endpoint with the new root only:
   `openssl verify -CAfile "$NEW_PKI/ca.pem" <leaf>` and
   `openssl s_client -connect <host:port> -CAfile "$NEW_PKI/ca.pem" -verify_return_error </dev/null`.
7. Drop the old root from every bundle (manual, irreversible: true), then
   verify that a leaf from the old root now FAILS `openssl verify` against the
   deployed bundle.

**Never** print a private key, `cat` a key file, or commit any of this
material; keys stay outside the repo.

**Rollback:** until step 7, the bundle still trusts the old root: put the
old service certs back (`<svc>.crt.old`) and restart that service.
