# deploy/systemd — Kingdom service units for EAST (bare metal)

Systemd units for all Kingdom services that run natively on EAST.
WEST runs these via Docker (`docker compose`); EAST runs them via systemd.

Units are **installed but not enabled** — they do not start on boot.
Start them explicitly with `systemctl start <unit>`.

## Services

| Unit file                       | Port(s)       | Notes                              |
|---------------------------------|---------------|------------------------------------|
| `unheaded-wotan.service`        | 18000, 18001  | Start first — others depend on it  |
| `unheaded-timeguru.service`     | 19000         |                                    |
| `unheaded-architect.service`    | 19001         |                                    |
| `unheaded-captain.service`      | 19002         |                                    |
| `unheaded-micromanager.service` | 19003         |                                    |
| `unheaded-monad.service`        | 19004 (gRPC)  |                                    |
| `unheaded-sophia.service`       | 19005 (gRPC)  |                                    |
| `unheaded-dashboard.service`    | 16667         |                                    |
| `unheaded-kanban.service`       | 16668         |                                    |
| `unheaded-daemon.service`       | 17000, 17001  | Control plane / drift detection    |
| `unheaded-akira.service`        | 19100         | Health monitor                     |
| `huginn.service`                | 9110          | Host metrics agent (ADR-084)       |

## Install (on EAST)

```bash
# Copy units
sudo cp deploy/systemd/*.service /etc/systemd/system/

# huginn env (edit HUGINN_VM for EAST — push to WEST's VictoriaMetrics)
sudo cp deploy/systemd/huginn.env.example /etc/huginn.env
sudo sed -i 's|http://localhost:8428|http://192.168.13.1:8428|' /etc/huginn.env

# Create data dirs and unheaded user if not present
sudo useradd -r -s /sbin/nologin unheaded 2>/dev/null || true
for svc in wotan timeguru captain architect micromanager monad sophia dashboard kanban daemon akira; do
    sudo install -d -o unheaded -g unheaded /var/lib/unheaded/$svc
done

sudo systemctl daemon-reload
```

## Start order

```bash
sudo systemctl start unheaded-wotan
sudo systemctl start unheaded-timeguru unheaded-architect unheaded-captain \
    unheaded-micromanager unheaded-monad unheaded-sophia
sudo systemctl start unheaded-dashboard unheaded-kanban unheaded-daemon unheaded-akira
sudo systemctl start huginn
```

## Do NOT enable on boot

These units intentionally have no `systemctl enable`. EAST is a staging host;
services are started manually per session. If auto-start is ever needed, revisit
as a deliberate decision.

## Binary paths

Binaries are expected at `/usr/local/bin/<name>`, installed from WEST's local
APT repo (`apt install unheaded-<service>`). See `runbooks/infra/apt-repo-server.yaml`.

## Logs

```bash
sudo journalctl -u huginn -f
sudo journalctl -u unheaded-wotan -f
```
