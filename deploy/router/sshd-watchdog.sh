#!/bin/sh

PATH=/opt/bin:/opt/sbin:/sbin:/bin:/usr/sbin:/usr/bin

CONF=/opt/etc/config/dropbear.conf
INIT=/opt/etc/init.d/S51dropbear
LOG=/opt/var/log/sshd-watchdog.log
MAXLOG=131072

PORT=22
PIDFILE=/opt/var/run/dropbear.pid
[ -f "$CONF" ] && . "$CONF"

listening() {
	netstat -ltn 2>/dev/null | awk '{print $4}' | grep -qE ":${PORT}\$"
}

listening && exit 0

[ -f "$LOG" ] && [ "$(wc -c < "$LOG")" -gt "$MAXLOG" ] && mv -f "$LOG" "$LOG.1"

{
	echo "--- $(date '+%F %T') dropbear not listening on $PORT"
	echo "pidfile: $([ -f "$PIDFILE" ] && cat "$PIDFILE" || echo none), pidof: $(pidof dropbear)"
	free | sed -n 2p
	uptime
} >> "$LOG" 2>&1

killall dropbear 2>/dev/null
rm -f "$PIDFILE"
sleep 1
"$INIT" start >> "$LOG" 2>&1
sleep 2

if listening; then
	echo "restarted ok, pid $(cat "$PIDFILE" 2>/dev/null)" >> "$LOG"
else
	echo "RESTART FAILED" >> "$LOG"
fi
