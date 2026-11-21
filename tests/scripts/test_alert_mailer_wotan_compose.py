# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""alert-mailer's link to Wotan (alerts.critical, ADR-147 Phase 8) is switched
on by configuration only: monitoring/docker-compose.yml gives the container
the service key, the signing key, the CA and the server name, every host
value comes from monitoring/.env through ${VAR:-default}, and with nothing
set the feature is off and the stack still starts.

The monitoring stack is started with `docker compose -f
monitoring/docker-compose.yml`, so no override file is read: what is not in
the file as an interpolation cannot be set on a host."""
import pathlib
import re

import yaml

REPO = pathlib.Path(__file__).resolve().parents[2]
COMPOSE = REPO / "monitoring/docker-compose.yml"
SECRETS = "/run/secrets/alert-mailer"
INTERPOLATION = re.compile(r"\$\{([A-Z0-9_]+)([^}]*)\}")


def mailer():
    return yaml.safe_load(COMPOSE.read_text(encoding="utf-8"))["services"]["alert-mailer"]


def mailer_text():
    """The alert-mailer entry as written, comments included."""
    text = COMPOSE.read_text(encoding="utf-8")
    start = text.index("\n  alert-mailer:\n")
    end = text.index("\n  # No node-exporter container", start)
    return text[start:end]


def mounts():
    """target -> (source as written, mode) of the entry's bind mounts."""
    out = {}
    for v in mailer()["volumes"]:
        m = re.fullmatch(r"(\$\{[^}]+\}|[^:]+):([^:]+)(?::(\w+))?", v)
        assert m, v
        out[m.group(2)] = (m.group(1), m.group(3))
    return out


def default_of(source):
    """The default of a ${VAR:-default} source, or the source itself."""
    m = INTERPOLATION.fullmatch(source)
    if not m:
        return source
    assert m.group(2).startswith(":-"), f"{source}: a host value without a default stops a stack that sets nothing"
    return m.group(2)[2:]


def test_off_until_the_address_is_set():
    env = mailer()["environment"]
    assert env["WOTAN_ADDR"] == "${ALERT_MAILER_WOTAN_ADDR:-}"
    assert env["WOTAN_GRPC_ADDR"] == "${ALERT_MAILER_WOTAN_GRPC_ADDR:-}"


def test_keys_come_from_the_mailers_own_secrets_directory():
    env = mailer()["environment"]
    assert env["AUTH_KEY_FILE"] == f"{SECRETS}/alert-mailer.key"
    assert env["ALERTS_SIGNING_KEY_FILE"] == f"{SECRETS}/alerts-critical.key"
    source, mode = mounts()[SECRETS]
    assert mode == "ro"
    assert source == "${ALERT_MAILER_SECRETS_DIR:-./alert-mailer/no-secrets}"
    # A compose secret names a file that must exist, so a host that has not
    # enrolled could not start the stack.
    assert "secrets" not in mailer()


def test_tls_to_wotan_with_the_ca_and_the_server_name():
    env = mailer()["environment"]
    # On unless a host turns it off: the link crosses the host's overlay
    # address, where a plaintext client withholds its token.
    assert env["UNHEADED_TLS_ENABLED"] == "${ALERT_MAILER_WOTAN_TLS:-true}"
    assert env["WOTAN_TLS_SERVER_NAME"] == "${ALERT_MAILER_WOTAN_TLS_SERVER_NAME:-wotan}"
    ca = env["UNHEADED_TLS_CA_CERT"]
    source, mode = mounts()[ca]
    assert mode == "ro"
    assert source == "${ALERT_MAILER_WOTAN_CA_FILE:-../configs/certs/dev/ca.pem}"


def test_the_ca_default_is_the_file_prometheus_already_mounts():
    """The default CA path adds no new requirement to a host: the stack's
    Prometheus mounts the same file for its https scrapes."""
    prom = yaml.safe_load(COMPOSE.read_text(encoding="utf-8"))["services"]["prometheus"]["volumes"]
    assert any(v.startswith("../configs/certs/dev/ca.pem:") for v in prom)


def test_the_mailers_own_listeners_get_no_certificate():
    """UNHEADED_TLS_ENABLED is for the Wotan client. The listeners stay plain
    HTTP (cmd/alert-mailer TestListenersDoNotReadTheTLSVariable), and what
    calls them speaks http."""
    env = mailer()["environment"]
    for name in ("UNHEADED_TLS_CERT", "UNHEADED_TLS_KEY_FILE", "UNHEADED_TLS_MTLS_ENABLED"):
        assert name not in env
    assert "http://127.0.0.1:19130/health" in mailer()["healthcheck"]["test"]
    am = (REPO / "monitoring/alertmanager/alertmanager.yml").read_text(encoding="utf-8")
    urls = re.findall(r"url:\s*['\"]?(\S+alert-mailer\S+?)['\"]?\s*$", am, flags=re.MULTILINE)
    assert urls, "alertmanager.yml names no alert-mailer webhook: this test checks nothing"
    for u in urls:
        assert u.startswith("http://alert-mailer:19130/"), u


def test_every_host_value_has_a_default_and_every_default_mount_exists():
    text = mailer_text()
    found = INTERPOLATION.findall(text)
    assert len(found) >= 10
    for name, rest in found:
        assert rest.startswith(":-"), f"${{{name}{rest}}}: no default"
    for target, (source, _) in mounts().items():
        default = default_of(source)
        if not default.startswith("."):
            continue  # a named volume
        path = (COMPOSE.parent / default).resolve()
        if default == "../configs/certs/dev/ca.pem":
            continue  # generated per checkout (scripts/gen-dev-certs.sh), as for Prometheus
        assert path.exists(), f"{target}: default source {default} is not in the repository"


def test_each_variable_is_documented():
    """A host sets these in monitoring/.env; each has a line in the wiki."""
    wiki = (REPO / "wiki/Alerting.md").read_text(encoding="utf-8")
    names = sorted({n for n, _ in INTERPOLATION.findall(mailer_text()) if n.startswith("ALERT_MAILER_")})
    assert "ALERT_MAILER_WOTAN_ADDR" in names
    for n in names:
        assert re.search(rf"\b{n}\b", wiki), f"{n} is not in wiki/Alerting.md"
