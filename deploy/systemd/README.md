# deploy/systemd — Kingdom service units for EAST (bare metal)

Systemd units for all Kingdom services that run natively on EAST.
WEST runs these via Docker (`docker compose`); EAST runs them via systemd.

## Boot policy

| Unit                            | Boot policy        | Rationale                                    |
|---------------------------------|--------------------|----------------------------------------------|
| `huginn.service`                | **enabled**        | Always-on; historical metrics need no gap    |
| `unheaded-victoria.service`     | **enabled**        | TSDB backing store for huginn                |
| All other units                 | disabled (manual)  | Dev/staging services, start per session      |

Grafana is NOT auto-started — resource cost isn't worth it when idle. Start it
manually with `sudo docker compose up grafana -d` when you want dashboards.

## Services

| Unit file                       | Port(s)       | Notes                              |
|---------------------------------|---------------|------------------------------------|
| `huginn.service`                | 9110          | Host metrics agent — **boot-enabled** (ADR-084) |
| `unheaded-victoria.service`     | 8428          | VictoriaMetrics TSDB — **boot-enabled**     |
| `unheaded-doom.target`          | —             | **DOOM pipeline target** — starts all 3 units below |
| `unheaded-doom-ring.service`    | —             | monad netns ring setup (one-shot)  |
| `unheaded-doom-runner.service`  | —             | Aya MBC executor + ROM + XDP attach |
| `unheaded-doom-injector.service`| —             | packet circulator (monad clock)    |
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

# Enable always-on services (huginn + victoria)
sudo systemctl enable --now huginn
sudo systemctl enable --now unheaded-victoria
```

## Manual session start (other services)

```bash
sudo systemctl start unheaded-wotan
sudo systemctl start unheaded-timeguru unheaded-architect unheaded-captain \
    unheaded-micromanager unheaded-monad unheaded-sophia
sudo systemctl start unheaded-dashboard unheaded-kanban unheaded-daemon unheaded-akira
```

## DOOM on the UPC (full pipeline)

```bash
# Start everything: ring → runner (+ XDP attach) → injector
sudo systemctl start unheaded-doom.target

# Stop everything cleanly (ring teardown included)
sudo systemctl stop unheaded-doom.target

# Status
sudo systemctl status unheaded-doom-ring unheaded-doom-runner unheaded-doom-injector

# Verify DOOM is rendering (non-zero SCREEN_MAP entries)
MAP_ID=$(sudo bpftool map list | awk '/SCREEN_MAP/{print $1}' | tr -d ':')
sudo bpftool map dump id $MAP_ID 2>/dev/null | grep -c '"value"'
# Then open http://localhost:16666/
```

The XDP attach step (the known bug in doom-runner main.rs:429-436) is handled
automatically by `ExecStartPost=doom-xdp-attach.sh` in doom-runner.service.
No manual bpftool command needed.

The dashboard web GUI runs via Docker Compose (not this target):
```bash
cd ~/tmp/unheaded && sudo docker compose up dashboard-backend -d
```

## Grafana (optional, on demand)

```bash
cd ~/tmp/unheaded && sudo docker compose up grafana -d
# Stop when done:
cd ~/tmp/unheaded && sudo docker compose stop grafana
```

## Binary paths

Binaries are expected at `/usr/local/bin/<name>`, installed from WEST's local
APT repo (`apt install unheaded-<service>`). See `runbooks/infra/apt-repo-server.yaml`.

## Logs

```bash
sudo journalctl -u huginn -f
sudo journalctl -u unheaded-wotan -f
```
