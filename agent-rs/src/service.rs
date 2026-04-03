use std::process::Command;

use tracing::{debug, error, info, warn};

use crate::keenetic;
use crate::models::*;

struct CmdResult {
    output: String,
}

fn run_cmd(cmd: &mut Command) -> Result<CmdResult, Error> {
    let cmd_str = format!(
        "{} {}",
        cmd.get_program().to_string_lossy(),
        cmd.get_args()
            .map(|a| a.to_string_lossy())
            .collect::<Vec<_>>()
            .join(" ")
    );
    debug!(cmd = %cmd_str, "command started");

    let start = std::time::Instant::now();
    let output = cmd.output().map_err(|e| {
        error!(cmd = %cmd_str, err = %e, "failed to execute command");
        Error {
            error: e.to_string(),
            exit_code: None,
            output: None,
        }
    })?;

    let exit_code = output.status.code().unwrap_or(-1);
    let stdout = String::from_utf8_lossy(&output.stdout).into_owned();
    let stderr = String::from_utf8_lossy(&output.stderr).into_owned();

    info!(cmd = %cmd_str, exit_code, duration_ms = start.elapsed().as_millis() as u64, "command executed");

    if output.status.success() {
        Ok(CmdResult { output: stdout })
    } else {
        Err(Error {
            error: format!("command failed with exit code {exit_code}"),
            exit_code: Some(exit_code),
            output: if stderr.is_empty() {
                None
            } else {
                Some(stderr)
            },
        })
    }
}

fn parse_output_lines(output: &str) -> Vec<&str> {
    output
        .lines()
        .map(|l| l.trim())
        .filter(|l| !l.is_empty())
        .collect()
}

pub fn has_rule(table: u32, iif: &str) -> Result<bool, Error> {
    let res = run_cmd(Command::new("ip").args(["rule", "list"]))?;

    let def = format!("from all iif {iif} lookup {table}");

    for line in parse_output_lines(&res.output) {
        if let Some((_, rest)) = line.split_once(':')
            && rest.trim() == def
        {
            return Ok(true);
        }
    }
    Ok(false)
}

pub fn add_rule(rule: &Rule) -> Result<(), Error> {
    let res = run_cmd(Command::new("ip").args([
        "rule",
        "add",
        "iif",
        &rule.iif,
        "table",
        &rule.table.to_string(),
        "priority",
        &rule.priority.to_string(),
    ]));

    match res {
        Ok(_) => {
            info!(table = rule.table, iif = %rule.iif, priority = rule.priority, "rule added");
            Ok(())
        }
        Err(e) => {
            error!(table = rule.table, iif = %rule.iif, err = %e.error, "failed to add rule");
            Err(e)
        }
    }
}

pub fn list_routes(table: u32) -> Result<Vec<Route>, Error> {
    let res = run_cmd(Command::new("ip").args(["route", "list", "table", &table.to_string()]))?;

    let lines = parse_output_lines(&res.output);
    let mut routes = Vec::with_capacity(lines.len());

    for line in lines {
        let parts: Vec<&str> = line.split_whitespace().collect();
        if parts.len() == 5 {
            routes.push(Route {
                table,
                iface: parts[2].to_string(),
                address: parts[0].to_string(),
            });
        } else {
            warn!(line, "unexpected route output");
        }
    }

    Ok(routes)
}

pub fn add_route(route: &Route) -> Result<(), Error> {
    let res = run_cmd(Command::new("ip").args([
        "route",
        "add",
        "table",
        &route.table.to_string(),
        &route.address,
        "dev",
        &route.iface,
    ]));

    match res {
        Ok(_) => {
            info!(table = route.table, address = %route.address, iface = %route.iface, "route added");
            Ok(())
        }
        Err(e) => {
            if e.output
                .as_deref()
                .is_some_and(|o| o.contains("RTNETLINK answers: File exists"))
            {
                info!(table = route.table, address = %route.address, iface = %route.iface, "route added (already existed)");
                return Ok(());
            }
            error!(table = route.table, address = %route.address, err = %e.error, "failed to add route");
            Err(e)
        }
    }
}

pub fn delete_route(route: &Route) -> Result<(), Error> {
    let res = run_cmd(Command::new("ip").args([
        "route",
        "del",
        "table",
        &route.table.to_string(),
        &route.address,
        "dev",
        &route.iface,
    ]));

    match res {
        Ok(_) => {
            info!(table = route.table, address = %route.address, iface = %route.iface, "route deleted");
            Ok(())
        }
        Err(e) => {
            error!(table = route.table, address = %route.address, err = %e.error, "failed to delete route");
            Err(e)
        }
    }
}

pub fn list_hosts() -> Result<Vec<HostInfo>, Error> {
    let res = run_cmd(Command::new("ndmc").args(["-c", "show device-list"]))?;

    let objs = keenetic::parse_output(&res.output).map_err(|e| {
        error!(err = %e, "failed to parse ndmc command output");
        Error {
            error: e,
            exit_code: None,
            output: None,
        }
    })?;

    let hosts = objs
        .iter()
        .filter_map(|obj| keenetic::get_object(obj, "host"))
        .map(parse_host_info)
        .collect();

    Ok(hosts)
}

fn parse_optional_obj<T>(
    obj: Option<&keenetic::Object>,
    parse_fn: fn(&keenetic::Object) -> T,
) -> Option<T> {
    obj.filter(|o| !o.is_empty()).map(parse_fn)
}

fn parse_host_info(obj: &keenetic::Object) -> HostInfo {
    HostInfo {
        mac: keenetic::get_string(obj, "mac"),
        via: keenetic::get_string(obj, "via"),
        ip: keenetic::get_string(obj, "ip"),
        hostname: keenetic::get_string(obj, "hostname"),
        name: keenetic::get_string(obj, "name"),
        registered: keenetic::get_bool(obj, "registered"),
        access: keenetic::get_string(obj, "access"),
        priority: keenetic::get_int(obj, "priority"),
        active: keenetic::get_bool(obj, "active"),
        rx_bytes: keenetic::get_int(obj, "rxbytes"),
        tx_bytes: keenetic::get_int(obj, "txbytes"),
        link: keenetic::get_string(obj, "link"),
        uptime: keenetic::get_int(obj, "uptime"),
        first_seen: keenetic::get_int(obj, "first-seen"),
        last_seen: keenetic::get_int(obj, "last-seen"),
        auto_negotiation: keenetic::get_bool(obj, "auto-negotiation"),
        speed: keenetic::get_int(obj, "speed"),
        duplex: keenetic::get_bool(obj, "duplex"),
        port: keenetic::get_int(obj, "port"),
        system_mode: keenetic::get_string(obj, "system-mode"),
        http_port: keenetic::get_int(obj, "http-port"),
        http_host: keenetic::get_string(obj, "http-host"),
        region: keenetic::get_string(obj, "region"),
        description: keenetic::get_string(obj, "description"),
        firmware: keenetic::get_string(obj, "firmware"),
        interface: parse_optional_obj(keenetic::get_object(obj, "interface"), |o| HostInterface {
            id: keenetic::get_string(o, "id"),
            name: keenetic::get_string(o, "name"),
            description: keenetic::get_string(o, "description"),
        }),
        dhcp: parse_optional_obj(keenetic::get_object(obj, "dhcp"), |o| HostDHCP {
            expires: keenetic::get_int(o, "expires"),
        }),
        mws: parse_optional_obj(keenetic::get_object(obj, "mws"), |o| HostMWS {
            cid: keenetic::get_string(o, "cid"),
            ap: keenetic::get_string(o, "ap"),
            psm: keenetic::get_bool(o, "psm"),
            mld: keenetic::get_bool(o, "mld"),
            authenticated: keenetic::get_bool(o, "authenticated"),
            tx_rate: keenetic::get_int(o, "txrate"),
            uptime: keenetic::get_int(o, "uptime"),
            rssi: keenetic::get_int(o, "rssi"),
            mcs: keenetic::get_int(o, "mcs"),
            security: keenetic::get_string(o, "security"),
        }),
    }
}
