#!/bin/sh

CMD=$1
SERVICE_NAME=$2
PROGRAM=$3
PIDFILE=$4
BIN=$5
UPDATE_BIN=$6
LOGFILE=$7

CHECK_INTERVAL=5
PID=

ansi_white="\033[1;37m";
ansi_std="\033[m";
ansi_SERVICE_NAME="$ansi_white$SERVICE_NAME$ansi_std"

start()
{
  if is_running; then
    echo -e "$ansi_SERVICE_NAME already running"
    self_pid=$(cut -d' ' -f4 < /proc/self/stat)
    run_pid=$(pgrep -o -f "service_watchdog.sh start $SERVICE_NAME")
    if [ "$self_pid" = "$run_pid" ]; then
      echo -e "$ansi_SERVICE_NAME start watchdog"
      # no other watchdog started, keep running and watching
      watchdog &
    fi
  else
    log "starting"
    eval "$PROGRAM &"
    PID=$!
    echo "$PID" > "$PIDFILE"
    log "started (pid $PID)"
    if [ "$1" = "watch" ]; then
      watchdog &
    fi
  fi
}

status()
{
  if is_running; then
    echo -e "$ansi_SERVICE_NAME is running"
  else
    echo -e "$ansi_SERVICE_NAME not started"
  fi
}

stop()
{
  if [ -n "$PID" ]; then
    if is_running; then
      log "stopping"
      kill "$PID" 2>/dev/null
      while [ -d "/proc/${PID}" ]; do sleep 1; done
      log "stopped"
    fi
    rm "$PIDFILE"
    PID=
  fi
}

is_running()
{
  [ -n "$PID" ] && [ -d "/proc/${PID}" ]
}

update_pid()
{
  pid=$(cat "$PIDFILE" 2>/dev/null)
  if [ -n "$PID" ] && [ -n "$pid" ] && [ "$PID" != "$pid" ]; then
    log "pid updated $PID -> $pid"
  fi
  PID=$pid
}

watchdog()
{
  log "watchdog started"
  while true; do
    update_pid
    if [ -n "$UPDATE_BIN" ] && [ -n "$BIN" ] && [ "$UPDATE_BIN" -nt "$BIN" -o ! -f "$BIN" ]; then
      # update is ready
      log "update is ready"
      stop
      cp -f "$UPDATE_BIN" "$BIN"
      start
    fi
    if ! is_running; then
      if [ -f "$PIDFILE" ]; then
        # pid file exists, looks like program exited unexpectedly
        log "program exited unexpectedly"
        stop
        start
      else
        # pid file not found, assume that program stopped
        log "exiting"
        exit
      fi
    fi
    sleep $CHECK_INTERVAL
  done
}

log()
{
  MSG=$1
  if [ -n "$LOGFILE" ] && [ -n "$MSG" ]; then
    echo "[$(date +"%Y-%m-%d %T")] $MSG" >> "$LOGFILE"
  else
    echo "$MSG"
  fi
}

update_pid

case "$CMD" in
  start)
    start watch
    ;;
  stop)
    stop
    ;;
  restart)
    stop
    start
    ;;
  status)
    status
    ;;
  *)
    echo "Usage: $0 {start|stop|restart|status}"
    exit 1
    ;;
esac
