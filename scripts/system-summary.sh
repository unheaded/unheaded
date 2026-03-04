#!/usr/bin/env bash
# /etc/profile.d/system-summary.sh
# Comprehensive system summary on login for admins

[[ $- == *i* ]] || return
[[ -n "${SYSTEM_SUMMARY_SHOWN:-}" ]] && return
export SYSTEM_SUMMARY_SHOWN=1

# Only show to root or sudo/wheel members
if [[ $EUID -ne 0 ]]; then
  id -nG "$USER" 2>/dev/null | tr ' ' '\n' | grep -qxE '(wheel|sudo)' || return
fi

# Load overrides
[[ -r /etc/sysconfig/system-summary ]] && source /etc/sysconfig/system-summary
[[ -r /etc/default/system-summary ]] && source /etc/default/system-summary

: "${SYSTEM_SUMMARY_DISK_WARN:=89}"
: "${SYSTEM_SUMMARY_MAX_USERS:=5}"
: "${SYSTEM_SUMMARY_SHOW_UPDATES:=0}"
: "${SYSTEM_SUMMARY_SHOW_CERTS:=1}"
: "${SYSTEM_SUMMARY_CERT_WARN_DAYS:=14}"

# HAProxy summary
: "${SYSTEM_SUMMARY_SHOW_HAPROXY_OFFENDERS:=0}"
: "${SYSTEM_SUMMARY_HAPROXY_TOP:=15}"
: "${SYSTEM_SUMMARY_HAPROXY_MAX_LINES:=200000}"
: "${SYSTEM_SUMMARY_HAPROXY_BAD_REGEX:=^(400|401|403|404|405|408|413|414|429|444|499|500|502|503|504)$}"
# Whois lookup settings
: "${SYSTEM_SUMMARY_HAPROXY_WHOIS:=1}"
: "${SYSTEM_SUMMARY_WHOIS_CACHE_DIR:=/tmp/whois_cache}"
: "${SYSTEM_SUMMARY_WHOIS_CACHE_TTL:=86400}"  # 24 hours in seconds
: "${SYSTEM_SUMMARY_WHOIS_TIMEOUT:=5}"

if [[ -t 1 ]]; then
  RED=$'\033[31m'; GREEN=$'\033[32m'; CYAN=$'\033[36m'; YELLOW=$'\033[33m'
  BOLD=$'\033[1m'; RESET=$'\033[0m'
else
  RED=""; GREEN=""; CYAN=""; YELLOW=""; BOLD=""; RESET=""
fi

LINE="═══════════════════════════════════════════════════════════"

hr() { printf "%b\n" "${CYAN}${BOLD}${LINE}${RESET}"; }

human_kib() {
  local kib="${1:-0}"
  awk -v k="$kib" 'BEGIN{
    units[0]="KiB"; units[1]="MiB"; units[2]="GiB"; units[3]="TiB";
    u=0; val=k;
    while(val>=1024 && u<3){ val=val/1024; u++ }
    if(u==0) printf "%.0f%s", val, units[u];
    else printf "%.1f%s", val, units[u];
  }'
}

get_session_origin() {
  local origin
  origin="$(who -m 2>/dev/null | sed -n 's/.*(\(.*\)).*/\1/p' | head -n1)"
  if [[ -n "$origin" ]]; then
    printf "%s" "$origin"
    return
  fi
  if [[ -n "${SSH_CONNECTION:-}" ]]; then
    awk '{print $1}' <<<"${SSH_CONNECTION}"
    return
  fi
  echo "local"
}

get_last_login_line() {
  last -n 2 "$USER" 2>/dev/null | awk 'NR==2 && !/^$|wtmp/ {print $3, $4, $5, $6}'
}

list_ports() {
  ss -tulnpH 2>/dev/null \
    | awk '
      {
        proto = ($1 ~ /udp/) ? "udp" : "tcp"
        local = ($1 ~ /udp/) ? $5 : $5
        split(local, a, ":")
        port = a[length(a)]
        proc = $0
        gsub(/.*users:\(\("?/, "", proc)
        gsub(/".*/, "", proc)
        if (proc == "") proc = "-"
        if (port ~ /^[0-9]+$/) printf "  %-6s %-20s %s\n", port, proc, proto
      }' \
    | sort -n -k1,1 \
    | uniq
}

failed_ssh_today() {
  local n
  n="$(journalctl -u sshd --since today --no-pager 2>/dev/null | grep -cE 'Failed password|authentication failure|Invalid user' || true)"
  if [[ -z "$n" || "$n" == "0" ]]; then
    n="$(journalctl --since today --no-pager 2>/dev/null | grep -cE 'sshd.*(Failed password|authentication failure|Invalid user)' || true)"
  fi
  echo "${n:-0}"
}

# Whois lookup with caching
get_whois_info() {
  local ip="$1"
  local cache_file="${SYSTEM_SUMMARY_WHOIS_CACHE_DIR}/${ip}.whois"
  local now
  now=$(date +%s)

  # Create cache directory if it doesn't exist
  mkdir -p "$SYSTEM_SUMMARY_WHOIS_CACHE_DIR" 2>/dev/null

  # Check cache
  if [[ -f "$cache_file" ]]; then
    local cache_time
    cache_time=$(stat -c %Y "$cache_file" 2>/dev/null || stat -f %m "$cache_file" 2>/dev/null)
    if [[ -n "$cache_time" ]] && (( now - cache_time < SYSTEM_SUMMARY_WHOIS_CACHE_TTL )); then
      cat "$cache_file"
      return 0
    fi
  fi

  # Perform whois lookup with timeout
  local whois_data
  if ! whois_data=$(timeout "${SYSTEM_SUMMARY_WHOIS_TIMEOUT}s" whois "$ip" 2>/dev/null); then
    echo "LOOKUP_FAILED"
    return 1
  fi

  # Parse whois data and extract relevant fields
  local netrange cidr orgname city state country regdate updated

  # Try ARIN format first
  netrange=$(echo "$whois_data" | grep -i "^NetRange:" | head -1 | sed 's/^[^:]*:[[:space:]]*//')
  cidr=$(echo "$whois_data" | grep -i "^CIDR:" | head -1 | sed 's/^[^:]*:[[:space:]]*//')
  orgname=$(echo "$whois_data" | grep -i "^OrgName:" | head -1 | sed 's/^[^:]*:[[:space:]]*//')
  city=$(echo "$whois_data" | grep -i "^City:" | head -1 | sed 's/^[^:]*:[[:space:]]*//')
  state=$(echo "$whois_data" | grep -i "^StateProv:" | head -1 | sed 's/^[^:]*:[[:space:]]*//')
  country=$(echo "$whois_data" | grep -i "^Country:" | head -1 | sed 's/^[^:]*:[[:space:]]*//')
  regdate=$(echo "$whois_data" | grep -i "^RegDate:" | head -1 | sed 's/^[^:]*:[[:space:]]*//')
  updated=$(echo "$whois_data" | grep -i "^Updated:" | head -1 | sed 's/^[^:]*:[[:space:]]*//')

  # Try RIPE/APNIC format if ARIN format didn't work
  if [[ -z "$netrange" ]]; then
    netrange=$(echo "$whois_data" | grep -i "^inetnum:" | head -1 | sed 's/^[^:]*:[[:space:]]*//')
  fi
  if [[ -z "$cidr" ]]; then
    cidr=$(echo "$whois_data" | grep -i "^route:" | head -1 | sed 's/^[^:]*:[[:space:]]*//')
  fi
  if [[ -z "$orgname" ]]; then
    orgname=$(echo "$whois_data" | grep -iE "^(org-name|descr|netname):" | head -1 | sed 's/^[^:]*:[[:space:]]*//')
  fi
  if [[ -z "$country" ]]; then
    country=$(echo "$whois_data" | grep -i "^country:" | head -1 | sed 's/^[^:]*:[[:space:]]*//')
  fi

  # Format output as pipe-delimited for easy parsing
  local result="${netrange:-N/A}|${cidr:-N/A}|${orgname:-N/A}|${city:-N/A}|${state:-N/A}|${country:-N/A}|${regdate:-N/A}|${updated:-N/A}"

  # Cache the result
  echo "$result" > "$cache_file" 2>/dev/null
  echo "$result"
}

haproxy_offenders() {
  local log="/var/log/haproxy.log"
  [[ -r "$log" ]] || return 0

  # Get list of offending IPs with counts
  local ip_data
  ip_data=$(tail -n "${SYSTEM_SUMMARY_HAPROXY_MAX_LINES}" "$log" 2>/dev/null \
    | awk -v TOP="${SYSTEM_SUMMARY_HAPROXY_TOP}" -v BAD="${SYSTEM_SUMMARY_HAPROXY_BAD_REGEX}" '
      {
        # Extract source IP - field 6 typically contains client:port
        src = $6
        gsub(/^[^0-9a-fA-F:.]+/, "", src)
        n = split(src, sp, ":")
        ip = sp[1]
        if (ip == "" || ip !~ /^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$/) next

        # Extract HTTP status code
        # HAProxy log format: ... status_code bytes_read ...
        status = ""
        for (i = 1; i <= NF; i++) {
          if ($i ~ /^[0-9]{3}$/ && $(i) >= 100 && $(i) <= 599) {
            status = $i
            break
          }
        }

        # Alternative: look for pattern like "200 +0" or "404 -1"
        if (status == "") {
          if (match($0, / ([0-9]{3}) [+-]?[0-9]+ /, arr)) {
            # gawk specific - use different approach
            temp = substr($0, RSTART+1, RLENGTH-2)
            split(temp, parts, " ")
            status = parts[1]
          }
        }

        # Count by IP
        total_by_ip[ip]++
        if (status ~ BAD) {
          bad_by_ip[ip]++
        }

        # Extract and count by host header
        host = ""
        if (match($0, /\{([^|{}]+)\|/, dummy)) {
          temp = substr($0, RSTART+1)
          sub(/\|.*/, "", temp)
          host = temp
        }
        if (host != "" && status ~ BAD) {
          bad_by_host[host]++
        }
      }
      END {
        # Sort IPs by bad count (selection sort)
        n = 0
        for (k in bad_by_ip) {
          n++
          keys[n] = k
        }
        for (i = 1; i <= n; i++) {
          max = i
          for (j = i + 1; j <= n; j++) {
            if (bad_by_ip[keys[j]] > bad_by_ip[keys[max]]) max = j
          }
          if (max != i) {
            tmp = keys[i]
            keys[i] = keys[max]
            keys[max] = tmp
          }
        }

        # Output top offenders (IP|bad|total format for parsing)
        shown = 0
        for (i = 1; i <= n && shown < TOP; i++) {
          k = keys[i]
          print k "|" bad_by_ip[k] "|" total_by_ip[k]
          shown++
        }
      }')

  if [[ -z "$ip_data" ]]; then
    printf "  Top bad clients (last %s log lines):\n" "${SYSTEM_SUMMARY_HAPROXY_MAX_LINES}"
    printf "    (none)\n"
  else
    printf "  Top bad clients (last %s log lines):\n\n" "${SYSTEM_SUMMARY_HAPROXY_MAX_LINES}"

    if (( SYSTEM_SUMMARY_HAPROXY_WHOIS == 1 )) && command -v whois >/dev/null 2>&1; then
      # Print header
      printf "    ${BOLD}%-16s  %-6s  %-6s  %-24s  %-20s  %-12s  %-3s  %-20s${RESET}\n" \
        "IP" "BAD" "TOTAL" "ORGANIZATION" "CIDR" "CITY" "CC" "NETRANGE"
      printf "    %s\n" "$(printf '─%.0s' {1..130})"

      while IFS='|' read -r ip bad total; do
        [[ -z "$ip" ]] && continue

        # Get whois info
        local whois_info
        whois_info=$(get_whois_info "$ip")

        if [[ "$whois_info" == "LOOKUP_FAILED" ]]; then
          printf "    %-16s  %-6s  %-6s  %-24s  %-20s  %-12s  %-3s  %-20s\n" \
            "$ip" "$bad" "$total" "(lookup failed)" "-" "-" "-" "-"
        else
          IFS='|' read -r netrange cidr orgname city state country regdate updated <<< "$whois_info"

          # Truncate long fields for display
          [[ ${#orgname} -gt 24 ]] && orgname="${orgname:0:21}..."
          [[ ${#cidr} -gt 20 ]] && cidr="${cidr:0:17}..."
          [[ ${#city} -gt 12 ]] && city="${city:0:9}..."
          [[ ${#netrange} -gt 20 ]] && netrange="${netrange:0:17}..."

          # Color code based on severity
          local color=""
          if (( bad > 500 )); then
            color="${RED}"
          elif (( bad > 100 )); then
            color="${YELLOW}"
          fi

          printf "    ${color}%-16s  %-6s  %-6s${RESET}  %-24s  %-20s  %-12s  %-3s  %-20s\n" \
            "$ip" "$bad" "$total" "${orgname:-N/A}" "${cidr:-N/A}" "${city:-N/A}" "${country:-N/A}" "${netrange:-N/A}"
        fi
      done <<< "$ip_data"

      printf "\n    ${CYAN}Additional Details (RegDate/Updated):${RESET}\n"
      while IFS='|' read -r ip bad total; do
        [[ -z "$ip" ]] && continue
        local whois_info
        whois_info=$(get_whois_info "$ip")
        if [[ "$whois_info" != "LOOKUP_FAILED" ]]; then
          IFS='|' read -r netrange cidr orgname city state country regdate updated <<< "$whois_info"
          if [[ "$regdate" != "N/A" || "$updated" != "N/A" ]]; then
            printf "      %-16s  RegDate: %-12s  Updated: %-12s  State: %s\n" \
              "$ip" "${regdate:-N/A}" "${updated:-N/A}" "${state:-N/A}"
          fi
        fi
      done <<< "$ip_data"
    else
      # Simple output without whois
      printf "    %-16s  %-6s  %-6s\n" "IP" "BAD" "TOTAL"
      printf "    %s\n" "$(printf '─%.0s' {1..35})"
      while IFS='|' read -r ip bad total; do
        [[ -z "$ip" ]] && continue
        printf "    %-16s  %-6s  %-6s\n" "$ip" "$bad" "$total"
      done <<< "$ip_data"

      if ! command -v whois >/dev/null 2>&1; then
        printf "\n    ${YELLOW}Note: Install 'whois' package for organization/location info${RESET}\n"
      fi
    fi
  fi

  # Get host data separately
  printf "\n"
  tail -n "${SYSTEM_SUMMARY_HAPROXY_MAX_LINES}" "$log" 2>/dev/null \
    | awk -v TOP="${SYSTEM_SUMMARY_HAPROXY_TOP}" -v BAD="${SYSTEM_SUMMARY_HAPROXY_BAD_REGEX}" '
      {
        status = ""
        for (i = 1; i <= NF; i++) {
          if ($i ~ /^[0-9]{3}$/ && $(i) >= 100 && $(i) <= 599) {
            status = $i
            break
          }
        }

        host = ""
        if (match($0, /\{([^|{}]+)\|/, dummy)) {
          temp = substr($0, RSTART+1)
          sub(/\|.*/, "", temp)
          host = temp
        }
        if (host != "" && status ~ BAD) {
          bad_by_host[host]++
        }
      }
      END {
        n = 0
        for (k in bad_by_host) {
          n++
          keys[n] = k
        }
        for (i = 1; i <= n; i++) {
          max = i
          for (j = i + 1; j <= n; j++) {
            if (bad_by_host[keys[j]] > bad_by_host[keys[max]]) max = j
          }
          if (max != i) {
            tmp = keys[i]
            keys[i] = keys[max]
            keys[max] = tmp
          }
        }
        print "  Top hosts hit with bad requests:"
        shown = 0
        for (i = 1; i <= n && shown < TOP; i++) {
          printf "    %-40s bad=%d\n", keys[i], bad_by_host[keys[i]]
          shown++
        }
        if (shown == 0) print "    (none)"
      }'
}

# ============================================================================
# BEGIN OUTPUT
# ============================================================================

printf "\n"
hr
printf "%b\n" "${CYAN}${BOLD}                   System Profile Summary${RESET}"
hr
printf "\n"

printf "%b\n\n" "${GREEN}${BOLD}Hello, ${USER}!${RESET}"

# ----------------------------------------------------------------------------
# SYSTEM INFO
# ----------------------------------------------------------------------------
printf "%b\n" "${CYAN}${BOLD}── System ──${RESET}"
printf "%b\n" "${GREEN}${BOLD}Date:${RESET}       $(date '+%Y-%m-%d %H:%M:%S %Z')"
printf "%b\n" "${GREEN}${BOLD}Host:${RESET}       $(hostname -f 2>/dev/null || hostname)"
printf "%b\n" "${GREEN}${BOLD}Kernel:${RESET}     $(uname -r)"

if command -v ip >/dev/null 2>&1; then
  IP4=$(ip -4 -o addr show scope global 2>/dev/null | awk '{print $2": "$4}' | paste -sd', ' -)
  [[ -n "$IP4" ]] && printf "%b\n" "${GREEN}${BOLD}IPv4:${RESET}       ${IP4}"
  IP6=$(ip -6 -o addr show scope global 2>/dev/null | awk '{print $2": "$4}' | paste -sd', ' -)
  [[ -n "$IP6" ]] && printf "%b\n" "${GREEN}${BOLD}IPv6:${RESET}       ${IP6}"
fi

ORIGIN="$(get_session_origin)"
printf "%b\n" "${GREEN}${BOLD}Session:${RESET}    from ${ORIGIN}"

printf "%b\n" "${GREEN}${BOLD}Uptime:${RESET}     $(uptime -p 2>/dev/null | sed 's/^up //')"
printf "%b\n" "${GREEN}${BOLD}Load:${RESET}       $(awk '{print $1, $2, $3}' /proc/loadavg 2>/dev/null) (1/5/15 min)"

# Virtualization
if command -v systemd-detect-virt >/dev/null 2>&1; then
  VIRT=$(systemd-detect-virt 2>/dev/null)
  [[ -n "$VIRT" && "$VIRT" != "none" ]] && printf "%b\n" "${GREEN}${BOLD}Virt:${RESET}       ${VIRT}"
fi

LAST="$(get_last_login_line)"
[[ -n "$LAST" ]] && printf "%b\n" "${GREEN}${BOLD}Last login:${RESET} ${LAST}"
echo

# ----------------------------------------------------------------------------
# USERS
# ----------------------------------------------------------------------------
printf "%b\n" "${CYAN}${BOLD}── Users Logged In ──${RESET}"
if command -v w >/dev/null 2>&1; then
  w -h 2>/dev/null | awk '
    {
      user=$1; tty=$2; from=$3; login=$4; idle=$5; what=$NF
      if(from ~ /^\(|^$/){ from="local"; login=$3; idle=$4 }
      printf "  %-12s %-8s %-16s idle=%-6s %s\n", user, tty, from, idle, what
    }' | head -n "$SYSTEM_SUMMARY_MAX_USERS"
else
  who 2>/dev/null | awk '{printf "  %-12s %-10s from %s\n", $1, $3, $5}' | head -n "$SYSTEM_SUMMARY_MAX_USERS"
fi
TOTAL_USERS=$(who 2>/dev/null | wc -l)
printf "  ${GREEN}Total: ${TOTAL_USERS} session(s)${RESET}\n"
echo

# ----------------------------------------------------------------------------
# MEMORY
# ----------------------------------------------------------------------------
printf "%b\n" "${CYAN}${BOLD}── Memory ──${RESET}"
if [[ -r /proc/meminfo ]]; then
  mem_total=$(awk '/MemTotal:/ {print $2}' /proc/meminfo)
  mem_avail=$(awk '/MemAvailable:/ {print $2}' /proc/meminfo)
  mem_free=$(awk '/MemFree:/ {print $2}' /proc/meminfo)
  mem_buffers=$(awk '/Buffers:/ {print $2}' /proc/meminfo)
  mem_cached=$(awk '/^Cached:/ {print $2}' /proc/meminfo)
  mem_used=$((mem_total - mem_avail))
  mem_pct=$((mem_used * 100 / mem_total))
  swap_total=$(awk '/SwapTotal:/ {print $2}' /proc/meminfo)
  swap_free=$(awk '/SwapFree:/ {print $2}' /proc/meminfo)
  swap_used=$((swap_total - swap_free))
  [[ $swap_total -gt 0 ]] && swap_pct=$((swap_used * 100 / swap_total)) || swap_pct=0

  printf "  RAM:      %s / %s used (%d%%)\n" "$(human_kib "$mem_used")" "$(human_kib "$mem_total")" "$mem_pct"
  printf "  Buffers:  %s\n" "$(human_kib "$mem_buffers")"
  printf "  Cached:   %s\n" "$(human_kib "$mem_cached")"
  printf "  Swap:     %s / %s used (%d%%)\n" "$(human_kib "$swap_used")" "$(human_kib "$swap_total")" "$swap_pct"

  # High memory warning
  if (( mem_pct > 90 )); then
    printf "  %b\n" "${RED}${BOLD}⚠ RAM usage over 90%!${RESET}"
  fi
  if (( swap_pct > 50 )); then
    printf "  %b\n" "${YELLOW}${BOLD}⚠ Swap usage over 50%${RESET}"
  fi
fi
echo

# ----------------------------------------------------------------------------
# DISK
# ----------------------------------------------------------------------------
printf "%b\n" "${CYAN}${BOLD}── Disk Usage ──${RESET}"
df -hP -x tmpfs -x devtmpfs -x overlay 2>/dev/null | awk -v WARN="$SYSTEM_SUMMARY_DISK_WARN" -v RED="$RED" -v YELLOW="$YELLOW" -v RESET="$RESET" -v BOLD="$BOLD" '
  NR==1 { printf "  %-20s %6s %6s %6s %5s  %s\n", "Filesystem", "Size", "Used", "Avail", "Use%", "Mounted" }
  NR>1 {
    pct=$5; gsub(/%/,"",pct)
    color=""
    if (pct+0 > WARN) color=RED BOLD
    else if (pct+0 > 75) color=YELLOW
    printf "  %s%-20s %6s %6s %6s %5s  %s%s\n", color, $1, $2, $3, $4, $5, $6, (color!="" ? RESET : "")
  }'

# Inode usage
INODE_WARN=0
while read -r ipct mount; do
  [[ "$ipct" =~ ^[0-9]+$ ]] || continue
  if (( ipct > 90 )); then
    (( INODE_WARN == 0 )) && printf "\n  %b\n" "${RED}${BOLD}⚠ INODE USAGE CRITICAL:${RESET}"
    INODE_WARN=1
    printf "    %s at %d%% inodes\n" "$mount" "$ipct"
  fi
done < <(df -iP -x tmpfs -x devtmpfs 2>/dev/null | awk 'NR>1 {gsub(/%/,"",$5); print $5, $6}')
echo

# ----------------------------------------------------------------------------
# LISTENING PORTS
# ----------------------------------------------------------------------------
printf "%b\n" "${CYAN}${BOLD}── Listening Ports ──${RESET}"
list_ports
LISTEN_COUNT=$(ss -tuln 2>/dev/null | grep -c LISTEN)
printf "  ${GREEN}Total: ${LISTEN_COUNT} port(s)${RESET}\n"
echo

# ----------------------------------------------------------------------------
# SYSTEMD SERVICES
# ----------------------------------------------------------------------------
printf "%b\n" "${CYAN}${BOLD}── Systemd Services ──${RESET}"
RUNNING=$(systemctl list-units --type=service --state=running --no-legend --plain 2>/dev/null | wc -l)
FAILED=$(systemctl list-units --type=service --state=failed --no-legend --plain 2>/dev/null | wc -l)
TOTAL=$(systemctl list-units --type=service --no-legend --plain 2>/dev/null | wc -l)
printf "  Running: %d | Failed: %d | Total loaded: %d\n" "$RUNNING" "$FAILED" "$TOTAL"

if (( FAILED > 0 )); then
  printf "\n  %b\n" "${RED}${BOLD}⚠ FAILED SERVICES:${RESET}"
  systemctl list-units --type=service --state=failed --no-legend --plain 2>/dev/null | while read -r unit _; do
    printf "    ${RED}✗${RESET} %s\n" "$unit"
    # Show brief status
    systemctl status "$unit" --no-pager 2>/dev/null | awk 'NR<=3 {print "      " $0}'
  done
fi
echo

# ----------------------------------------------------------------------------
# SYSTEMD TIMERS
# ----------------------------------------------------------------------------
printf "%b\n" "${CYAN}${BOLD}── Upcoming Timers ──${RESET}"
systemctl list-timers --all --no-pager 2>/dev/null | awk '
  NR==1 { next }
  NR>1 && NR<=6 && !/timers listed/ && NF>2 {
    printf "  %s %s %s → %s\n", $1, $2, $3, $NF
  }'
echo

# ----------------------------------------------------------------------------
# NTP / TIME SYNC
# ----------------------------------------------------------------------------
printf "%b\n" "${CYAN}${BOLD}── Time Synchronization ──${RESET}"
if command -v timedatectl >/dev/null 2>&1; then
  NTP_SYNC=$(timedatectl show -p NTPSynchronized --value 2>/dev/null)
  NTP_SVC=$(timedatectl show -p NTP --value 2>/dev/null)
  TZ=$(timedatectl show -p Timezone --value 2>/dev/null)
  printf "  Timezone:     %s\n" "$TZ"
  printf "  NTP service:  %s\n" "$NTP_SVC"
  if [[ "$NTP_SYNC" == "yes" ]]; then
    printf "  Synchronized: ${GREEN}yes${RESET}\n"
  else
    printf "  Synchronized: ${RED}${BOLD}NO - TIME MAY BE WRONG${RESET}\n"
  fi
  # Show current offset if chrony is available
  if command -v chronyc >/dev/null 2>&1; then
    OFFSET=$(chronyc tracking 2>/dev/null | awk '/System time/ {print $4, $5}')
    [[ -n "$OFFSET" ]] && printf "  Offset:       %s\n" "$OFFSET"
  fi
fi
echo

# ----------------------------------------------------------------------------
# KERNEL / REBOOT
# ----------------------------------------------------------------------------
printf "%b\n" "${CYAN}${BOLD}── Kernel Status ──${RESET}"
CURRENT_KERNEL=$(uname -r)
if command -v rpm >/dev/null 2>&1; then
  LATEST_KERNEL=$(rpm -q kernel --last 2>/dev/null | head -1 | awk '{print $1}' | sed 's/^kernel-//')
  printf "  Running:   %s\n" "$CURRENT_KERNEL"
  printf "  Installed: %s\n" "$LATEST_KERNEL"
  if [[ -n "$LATEST_KERNEL" && "$CURRENT_KERNEL" != "$LATEST_KERNEL" ]]; then
    printf "  %b\n" "${RED}${BOLD}⚠ REBOOT REQUIRED to load new kernel${RESET}"
  else
    printf "  ${GREEN}Kernel is current${RESET}\n"
  fi
fi

# Check for needs-restarting if available (dnf-utils)
if command -v needs-restarting >/dev/null 2>&1; then
  NEED_RESTART=$(needs-restarting -r 2>/dev/null; echo $?)
  if [[ "$NEED_RESTART" == "1" ]]; then
    printf "  %b\n" "${YELLOW}${BOLD}⚠ Services need restart after updates${RESET}"
  fi
fi
echo

# ----------------------------------------------------------------------------
# SECURITY
# ----------------------------------------------------------------------------
printf "%b\n" "${CYAN}${BOLD}── Security ──${RESET}"

# SELinux
if command -v getenforce >/dev/null 2>&1; then
  SELINUX=$(getenforce 2>/dev/null)
  case "$SELINUX" in
    Enforcing) printf "  SELinux:       ${GREEN}Enforcing${RESET}\n" ;;
    Permissive) printf "  SELinux:       ${YELLOW}Permissive${RESET}\n" ;;
    Disabled) printf "  SELinux:       ${RED}${BOLD}Disabled${RESET}\n" ;;
  esac
fi

# Firewalld
if command -v firewall-cmd >/dev/null 2>&1; then
  FW_STATE=$(firewall-cmd --state 2>/dev/null)
  if [[ "$FW_STATE" == "running" ]]; then
    FW_ZONE=$(firewall-cmd --get-default-zone 2>/dev/null)
    printf "  Firewalld:     ${GREEN}running${RESET} (zone: %s)\n" "$FW_ZONE"
  else
    printf "  Firewalld:     ${RED}${BOLD}not running${RESET}\n"
  fi
fi

# Failed SSH
FAILS="$(failed_ssh_today)"
printf "  SSH failures:  %s today" "$FAILS"
if (( FAILS > 100 )); then
  printf " ${RED}${BOLD}(high!)${RESET}"
elif (( FAILS > 20 )); then
  printf " ${YELLOW}(elevated)${RESET}"
fi
printf "\n"

# Last 5 failed SSH IPs
if (( FAILS > 0 )); then
  printf "  Recent failed IPs:\n"
  journalctl -u sshd --since today --no-pager 2>/dev/null \
    | grep -oE 'from [0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' \
    | sort | uniq -c | sort -rn | head -5 \
    | awk '{printf "    %-16s %d attempts\n", $3, $1}'
fi

# fail2ban
if command -v fail2ban-client >/dev/null 2>&1; then
  F2B_STATUS=$(fail2ban-client status 2>/dev/null)
  if [[ -n "$F2B_STATUS" ]]; then
    JAILS=$(echo "$F2B_STATUS" | awk -F: '/Jail list/ {gsub(/,? /,"\n",$2); print $2}')
    TOTAL_BANNED=0
    printf "  fail2ban:      ${GREEN}active${RESET}\n"
    for jail in $JAILS; do
      [[ -z "$jail" ]] && continue
      BANNED=$(fail2ban-client status "$jail" 2>/dev/null | awk -F: '/Currently banned/ {gsub(/ /,"",$2); print $2}')
      TOTAL=$(fail2ban-client status "$jail" 2>/dev/null | awk -F: '/Total banned/ {gsub(/ /,"",$2); print $2}')
      printf "    Jail %-12s: %s banned (total: %s)\n" "$jail" "${BANNED:-0}" "${TOTAL:-0}"
      TOTAL_BANNED=$((TOTAL_BANNED + BANNED))
    done
    if (( TOTAL_BANNED > 0 )); then
      printf "  %b\n" "${YELLOW}🚫 ${TOTAL_BANNED} IP(s) currently banned${RESET}"
    fi
  fi
else
  printf "  fail2ban:      ${YELLOW}not installed${RESET}\n"
fi
echo

# ----------------------------------------------------------------------------
# SSL CERTIFICATES
# ----------------------------------------------------------------------------
if (( SYSTEM_SUMMARY_SHOW_CERTS == 1 )) && [[ -d /etc/letsencrypt/live ]]; then
  printf "%b\n" "${CYAN}${BOLD}── SSL Certificates ──${RESET}"
  WARN_SECS=$((SYSTEM_SUMMARY_CERT_WARN_DAYS * 86400))
  for cert in /etc/letsencrypt/live/*/cert.pem; do
    [[ -f "$cert" ]] || continue
    domain=$(basename "$(dirname "$cert")")
    expiry_date=$(openssl x509 -enddate -noout -in "$cert" 2>/dev/null | cut -d= -f2)
    expiry_epoch=$(date -d "$expiry_date" +%s 2>/dev/null)
    now_epoch=$(date +%s)
    days_left=$(( (expiry_epoch - now_epoch) / 86400 ))

    if (( days_left < 0 )); then
      printf "  ${RED}${BOLD}✗${RESET} %-30s ${RED}${BOLD}EXPIRED %d days ago${RESET}\n" "$domain" "$((-days_left))"
    elif (( days_left < SYSTEM_SUMMARY_CERT_WARN_DAYS )); then
      printf "  ${YELLOW}⚠${RESET} %-30s ${YELLOW}expires in %d days${RESET} (%s)\n" "$domain" "$days_left" "$expiry_date"
    else
      printf "  ${GREEN}✓${RESET} %-30s expires in %d days (%s)\n" "$domain" "$days_left" "$expiry_date"
    fi
  done
  echo
fi

# ----------------------------------------------------------------------------
# PROCESSES
# ----------------------------------------------------------------------------
printf "%b\n" "${CYAN}${BOLD}── Process Summary ──${RESET}"
TOTAL_PROCS=$(ps aux 2>/dev/null | wc -l)
ZOMBIES=$(ps -eo stat 2>/dev/null | awk '/^Z/ {c++} END{print c+0}')
printf "  Total processes: %d\n" "$TOTAL_PROCS"
printf "  Zombie processes: %d" "$ZOMBIES"
if (( ZOMBIES > 0 )); then
  printf " ${YELLOW}${BOLD}⚠${RESET}\n"
  printf "  Zombie details:\n"
  ps aux 2>/dev/null | awk '$8 ~ /^Z/ {printf "    PID=%-6s PPID=%-6s CMD=%s\n", $2, $3, $11}' | head -5
else
  printf " ${GREEN}✓${RESET}\n"
fi

# Top CPU consumers
printf "  Top CPU:\n"
ps aux --sort=-%cpu 2>/dev/null | awk 'NR>1 && NR<=4 {printf "    %-15s %5s%% CPU  %s\n", $1, $3, $11}'

# Top memory consumers
printf "  Top Memory:\n"
ps aux --sort=-%mem 2>/dev/null | awk 'NR>1 && NR<=4 {printf "    %-15s %5s%% MEM  %s\n", $1, $4, $11}'
echo

# ----------------------------------------------------------------------------
# DOCKER
# ----------------------------------------------------------------------------
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  printf "%b\n" "${CYAN}${BOLD}── Docker ──${RESET}"
  RUNNING=$(docker ps -q 2>/dev/null | wc -l)
  STOPPED=$(docker ps -aq --filter "status=exited" 2>/dev/null | wc -l)
  PAUSED=$(docker ps -aq --filter "status=paused" 2>/dev/null | wc -l)
  IMAGES=$(docker images -q 2>/dev/null | wc -l)

  printf "  Containers: %d running, %d stopped, %d paused\n" "$RUNNING" "$STOPPED" "$PAUSED"
  printf "  Images: %d\n" "$IMAGES"

  if (( RUNNING > 0 )); then
    printf "  Running containers:\n"
    docker ps --format "    {{.Names}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null | head -10
  fi

  # Docker disk usage
  DOCKER_SIZE=$(docker system df 2>/dev/null | awk 'NR==2 {print $4}')
  [[ -n "$DOCKER_SIZE" ]] && printf "  Disk used: %s\n" "$DOCKER_SIZE"
  echo
fi

# ----------------------------------------------------------------------------
# PACKAGE UPDATES
# ----------------------------------------------------------------------------
if (( SYSTEM_SUMMARY_SHOW_UPDATES == 1 )) && command -v dnf >/dev/null 2>&1; then
  printf "%b\n" "${CYAN}${BOLD}── Package Updates ──${RESET}"
  UPDATES=$(timeout 5s dnf -q check-update 2>/dev/null | awk 'NF && /^[A-Za-z0-9]/ {c++} END{print c+0}')
  if [[ "$UPDATES" =~ ^[0-9]+$ ]] && (( UPDATES > 0 )); then
    printf "  %b\n" "${YELLOW}${BOLD}📦 ${UPDATES} package update(s) available${RESET}"
    # Show security updates if possible
    SEC_UPDATES=$(timeout 5s dnf -q updateinfo list security 2>/dev/null | wc -l)
    (( SEC_UPDATES > 0 )) && printf "  %b\n" "${RED}${BOLD}🔒 ${SEC_UPDATES} security update(s)${RESET}"
  else
    printf "  ${GREEN}System is up to date${RESET}\n"
  fi
  echo
fi

# ----------------------------------------------------------------------------
# HAPROXY OFFENDERS (optional)
# ----------------------------------------------------------------------------
if [[ "${SYSTEM_SUMMARY_SHOW_HAPROXY_OFFENDERS}" == "1" ]]; then
  printf "%b\n" "${CYAN}${BOLD}── HAProxy Offenders ──${RESET}"
  haproxy_offenders
  echo
fi

# ----------------------------------------------------------------------------
# NETWORK CONNECTIONS SUMMARY
# ----------------------------------------------------------------------------
printf "%b\n" "${CYAN}${BOLD}── Network Connections ──${RESET}"
ESTAB=$(ss -tn state established 2>/dev/null | wc -l)
TIME_WAIT=$(ss -tn state time-wait 2>/dev/null | wc -l)
CLOSE_WAIT=$(ss -tn state close-wait 2>/dev/null | wc -l)
printf "  Established: %d | TIME_WAIT: %d | CLOSE_WAIT: %d\n" "$ESTAB" "$TIME_WAIT" "$CLOSE_WAIT"
if (( CLOSE_WAIT > 50 )); then
  printf "  %b\n" "${YELLOW}${BOLD}⚠ High CLOSE_WAIT count may indicate connection issues${RESET}"
fi
if (( TIME_WAIT > 1000 )); then
  printf "  %b\n" "${YELLOW}${BOLD}⚠ High TIME_WAIT count${RESET}"
fi
echo

# ----------------------------------------------------------------------------
# RECENT SYSTEM ERRORS
# ----------------------------------------------------------------------------
printf "%b\n" "${CYAN}${BOLD}── Recent Critical/Error Logs (last hour) ──${RESET}"
ERROR_COUNT=$(journalctl -p err --since "1 hour ago" --no-pager 2>/dev/null | grep -v "^-- " | wc -l)
if (( ERROR_COUNT > 0 )); then
  printf "  ${YELLOW}Found %d error-level messages${RESET}\n" "$ERROR_COUNT"
  printf "  Most recent:\n"
  journalctl -p err --since "1 hour ago" --no-pager -n 5 2>/dev/null | grep -v "^-- " | while read -r line; do
    printf "    %s\n" "${line:0:100}"
  done
else
  printf "  ${GREEN}No critical/error messages in the last hour${RESET}\n"
fi
echo

# ----------------------------------------------------------------------------
# QUICK COMMANDS REMINDER
# ----------------------------------------------------------------------------
printf "%b\n" "${CYAN}${BOLD}── Quick Commands ──${RESET}"
printf "  ${GREEN}htop${RESET}                    - interactive process viewer\n"
printf "  ${GREEN}journalctl -f${RESET}           - follow system logs\n"
printf "  ${GREEN}systemctl --failed${RESET}      - show failed services\n"
printf "  ${GREEN}ss -tulnp${RESET}               - show listening ports\n"
printf "  ${GREEN}df -h${RESET}                   - disk usage\n"
printf "  ${GREEN}free -h${RESET}                 - memory usage\n"
echo

hr
printf "\n"
