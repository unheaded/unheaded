# kingdom-base

Installs `deploy/sysusers.d/unheaded.conf` and runs `systemd-sysusers`, so every
service account exists (own uid, `/usr/sbin/nologin`), and creates `/etc/unheaded`.

Variables: `unheaded_repo` (path to a checkout on the controller).

The other Kingdom roles assume it ran first.
