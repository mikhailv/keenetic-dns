#!/bin/sh

DIR="/opt/keenetic-dns"

PIDFILE="/opt/var/run/$SERVICE_NAME.pid"
BIN="$DIR/$SERVICE_NAME"
UPDATE_BIN="$DIR/update/$SERVICE_NAME"
OUT="$DIR/logs/$SERVICE_NAME.out"
ENV=${ENV:-}
PROGRAM="cd $DIR && $ENV $BIN $ARGS >> $OUT 2>&1"

WATCHDOG="$DIR/service_watchdog.sh"
WATCHDOG_LOG="$DIR/logs/$SERVICE_NAME-watchdog.log"

case "$1" in
  start | stop | restart | status)
    "$WATCHDOG" "$1" "$SERVICE_NAME" "$PROGRAM" "$PIDFILE" "$BIN" "$UPDATE_BIN" "$WATCHDOG_LOG"
    ;;
  *)
    echo "Usage: $0 {start|stop|restart|status}"
    exit 1
    ;;
esac

