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

note() {
	logger -t sshd-watchdog "$1"
	mkdir -p "$(dirname "$LOG")" 2>/dev/null
	echo "$(date '+%F %T') $1" >> "$LOG" 2>/dev/null
}

listening && exit 0

note "port $PORT down (pidfile $(cat "$PIDFILE" 2>/dev/null || echo none), pidof '$(pidof dropbear)', $(free | awk 'NR==2 {print $4}')kB free)"

killall dropbear 2>/dev/null
rm -f "$PIDFILE"
sleep 1
"$INIT" start > /dev/null 2>&1
sleep 2

if ! listening; then
	note "restart FAILED"
	exit 1
fi

note "restarted ok, pid $(cat "$PIDFILE" 2>/dev/null)"

if [ -f "$LOG" ] && [ "$(wc -c < "$LOG")" -gt "$MAXLOG" ]; then
	mv -f "$LOG" "$LOG.1"
fi
