#!/bin/sh

DIR="/opt/keenetic-dns"

PIDFILE="/opt/var/run/$SERVICE_NAME.pid"
BIN="$DIR/$SERVICE_NAME"
UPDATE_BIN="$DIR/update/$SERVICE_NAME"
LOG="$DIR/$SERVICE_NAME.log"
PROGRAM="cd $DIR && $BIN 2>&1 >> $LOG"

WATCHDOG="$DIR/service_watchdog.sh"
WATCHDOG_LOG="$DIR/$SERVICE_NAME-watchdog.log"

watchdog()
{
  "$WATCHDOG" "$1" "$PROGRAM" "$PIDFILE" "$BIN" "$UPDATE_BIN" "$WATCHDOG_LOG"
}

case "$1" in
  start)
    watchdog start &
    ;;

  stop)
    watchdog stop
    ;;

  restart)
    watchdog restart
    ;;

  status)
    watchdog status
    ;;

  *)
    echo "Usage: $0 {start|stop|restart|status}"
    ;;
esac

