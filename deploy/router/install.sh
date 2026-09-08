#!/bin/sh

set -e

DIR=$(cd "$(dirname "$0")" && pwd)
CRONTAB=/opt/var/spool/cron/crontabs/root

if ! opkg list-installed | grep -q '^cron '; then
	opkg update
	opkg install cron
fi

chmod +x "$DIR/sshd-watchdog.sh"

mkdir -p "$(dirname "$CRONTAB")"
cp -f "$DIR/crontab.root" "$CRONTAB"
chmod 600 "$CRONTAB"

/opt/etc/init.d/S10cron restart

"$DIR/sshd-watchdog.sh"
echo "sshd watchdog installed, cron: $(crontab -l | tail -1)"
