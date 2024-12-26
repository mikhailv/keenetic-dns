#!/bin/sh

CMD=$1
PROGRAM=$2
PIDFILE=$3
BIN=$4
UPDATE_BIN=$5
LOGFILE=$6

CHECK_INTERVAL=5
PID=

start()
{
  if is_running; then
    echo "already running"
  else
    log "starting"
    eval "$PROGRAM &"
    PID=$!
    echo "$PID" > "$PIDFILE"
    log "started (pid $PID)"
    if [ "$1" = "watch" ]; then
      watchdog
    fi
  fi
}

status()
{
  if is_running; then
    echo "started and running"
  else
    echo "not started"
  fi
}

stop()
{
  if [ -n "$PID" ]; then
    if is_running; then
      log "stopping"
      kill "$PID" 2>/dev/null
      log "stopped"
    fi
    rm "$PIDFILE"
    PID=
  fi
}

restart()
{
  stop
  start
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
    if [ -n "$UPDATE_BIN" ] && [ -n "$BIN" ] && [ "$UPDATE_BIN" -nt "$BIN" ]; then
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
        restart
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
    echo "$MSG" >> "$LOGFILE"
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
    restart
    ;;

  status)
    status
    ;;

  *)
    echo "Usage: $0 {start|stop|restart|status}"
    ;;
esac
