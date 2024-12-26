#!/bin/sh

ps | grep '[s]ervice_watchdog' | awk '{ print $1 }' | xargs kill

/opt/etc/init.d/S80-agent start
/opt/etc/init.d/S81-dns-server start
