# victoria

West only. Brings up the `victoria` service of the root `docker-compose.yml`
(VictoriaMetrics, host port 8428; huginn on both hosts and Prometheus write to it)
and waits for `/-/healthy`. Compose owns the container, its image pin and its
hardening; the role does not restate them.

Variables: `unheaded_repo_on_host` (the checkout on west; default
`/home/govan/tmp/unheaded`).
