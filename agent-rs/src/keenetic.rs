use std::collections::HashMap;

#[derive(Debug, Clone, PartialEq)]
pub enum Value {
    String(String),
    Object(Object),
}

pub type Object = HashMap<String, Value>;

pub fn get_string(obj: &Object, prop: &str) -> Option<String> {
    if let Some((before, after)) = prop.split_once('.') {
        if let Some(Value::Object(child)) = obj.get(before) {
            return get_string(child, after);
        }
        return None;
    }
    match obj.get(prop) {
        Some(Value::String(s)) => Some(s.clone()),
        _ => None,
    }
}

pub fn get_int(obj: &Object, prop: &str) -> Option<i64> {
    get_string(obj, prop).and_then(|s| s.parse().ok())
}

pub fn get_bool(obj: &Object, prop: &str) -> Option<bool> {
    get_string(obj, prop).map(|s| s == "yes")
}

pub fn get_object<'a>(obj: &'a Object, prop: &str) -> Option<&'a Object> {
    match obj.get(prop) {
        Some(Value::Object(o)) => Some(o),
        _ => None,
    }
}

pub fn parse_output(output: &str) -> Result<Vec<Object>, String> {
    let mut result: Vec<Object> = Vec::new();

    struct StackItem {
        indent: usize,
        path: Vec<String>,
    }

    let mut stack: Vec<StackItem> = Vec::new();

    for pair in iterate_line_pairs(iterate_parsed_lines(iterate_lines(output))) {
        let (cur, next) = pair?;
        // Pop stack items with deeper indent
        while let Some(top) = stack.last() {
            if top.indent > cur.indent {
                stack.pop();
            } else {
                break;
            }
        }

        if stack.is_empty() {
            if !cur.value.is_empty() {
                return Err(format!("[ndm] unexpected object line: {:?}", cur.raw));
            }
            if next.indent <= cur.indent {
                return Err(format!(
                    "[ndm] unexpected indent of lines: {:?} and {:?}",
                    cur.raw, next.raw
                ));
            }
            let mut obj = Object::new();
            obj.insert(cur.key.clone(), Value::Object(Object::new()));
            result.push(obj);
            stack.push(StackItem {
                indent: next.indent,
                path: vec![cur.key],
            });
            continue;
        }

        let parent = stack.last().unwrap();
        if parent.indent == cur.indent {
            let path = parent.path.clone();
            if next.indent > cur.indent {
                // New nested object
                if !cur.value.is_empty() {
                    return Err(format!(
                        "[ndm] unexpected start of new object: {:?} and {:?}",
                        cur.raw, next.raw
                    ));
                }
                let parent_obj = get_object_at_path_mut(result.last_mut().unwrap(), &path);
                parent_obj.insert(cur.key.clone(), Value::Object(Object::new()));
                let mut new_path = path;
                new_path.push(cur.key);
                stack.push(StackItem {
                    indent: next.indent,
                    path: new_path,
                });
            } else {
                // String value
                let parent_obj = get_object_at_path_mut(result.last_mut().unwrap(), &path);
                parent_obj.insert(cur.key, Value::String(cur.value));
            }
        }
    }

    Ok(result)
}

fn get_object_at_path_mut<'a>(root: &'a mut Object, path: &[String]) -> &'a mut Object {
    let mut current = root;
    for key in path {
        current = match current.get_mut(key) {
            Some(Value::Object(o)) => o,
            _ => panic!("invalid path: key={key:?}"),
        };
    }
    current
}

#[derive(Debug, Clone)]
struct ParsedLine {
    raw: String,
    key: String,
    value: String,
    indent: usize, // position of ": " + 2 (matches Go)
}

fn iterate_lines(output: &str) -> impl Iterator<Item = &str> {
    output.split('\n').map(|line| line.trim_end_matches('\r'))
}

/// Parse raw lines into ParsedLine. Lines with `": "` get key/value/indent.
/// Lines without `": "` get key="", value="", indent=0 — they are treated as
/// continuation lines by iterate_line_pairs.
fn iterate_parsed_lines<'a>(
    lines: impl Iterator<Item = &'a str>,
) -> impl Iterator<Item = ParsedLine> {
    lines.filter_map(|line| {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed == "\x1b[K" {
            return None;
        }

        let mut parsed = ParsedLine {
            raw: line.to_string(),
            key: String::new(),
            value: String::new(),
            indent: 0,
        };

        if let Some(p) = line.find(": ")
            && p > 0
        {
            parsed.key = line[..p].trim().to_string();
            parsed.value = line[p + 2..].trim().to_string();
            parsed.indent = p + 2;
        }

        Some(parsed)
    })
}

/// Merges continuation lines and yields (current, next) pairs.
/// Continuation lines (key and value both empty) are appended to the previous line's value.
fn iterate_line_pairs(
    lines: impl Iterator<Item = ParsedLine>,
) -> LinePairs<impl Iterator<Item = ParsedLine>> {
    LinePairs {
        inner: lines,
        buf: None,
        done: false,
    }
}

struct LinePairs<I> {
    inner: I,
    buf: Option<ParsedLine>,
    done: bool,
}

impl<I: Iterator<Item = ParsedLine>> Iterator for LinePairs<I> {
    type Item = Result<(ParsedLine, ParsedLine), String>;

    fn next(&mut self) -> Option<Self::Item> {
        if self.done {
            return None;
        }

        // Initialize buffer with first line
        if self.buf.is_none() {
            self.buf = self.inner.next();
            if self.buf.is_none() {
                self.done = true;
                return None;
            }
        }

        loop {
            let Some(line2) = self.inner.next() else {
                // Last line — yield paired with itself
                self.done = true;
                let line1 = self.buf.take().unwrap();
                return Some(Ok((line1.clone(), line1)));
            };

            if line2.key.is_empty() && line2.value.is_empty() {
                // Continuation line
                let line1 = self.buf.as_mut().unwrap();
                if line2.raw.len() <= line1.indent || !line2.raw[..line1.indent].trim().is_empty() {
                    self.done = true;
                    return Some(Err(format!("[ndm] unexpected next line: {:?}", line2.raw)));
                }
                line1.value += line2.raw[line1.indent..].trim_end();
                continue;
            }

            let line1 = self.buf.replace(line2.clone()).unwrap();
            return Some(Ok((line1, line2)));
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // Real ndmc "show device-list" output (reduced).
    // Section headers have trailing space after ": " (e.g. "host: ").
    // Sibling keys are right-aligned so ": " is at the same column.
    const NDMC_OUTPUT: &str = r#"
             host: 
                  mac: dc:03:98:81:d1:04
                   ip: 192.168.2.18
             hostname: LGwebOSTV
                 name: LGwebOSTV

            interface: 
                       id: Bridge0
                     name: Home
              description: Home network

                 dhcp: 
                  expires: 4866

           registered: yes
               active: yes
              rxbytes: 280232502
              txbytes: 4729253

                  mws: 
                      cid: e0b91322-8299-11ea-8f23-9317f81e1607
                       ap: WifiMaster1/AccessPoint0
                      psm: no
                     rssi: -41

               uptime: 20335
           first-seen: 20330
            last-seen: 15

             host: 
                  mac: 70:89:76:dc:16:53
                   ip: 192.168.2.89
             hostname: 
                 name: 

            interface: 
                       id: Bridge0
                     name: Home

           registered: no
               active: yes
               uptime: 35622
"#;

    macro_rules! obj {
        ($($key:expr => $val:expr),* $(,)?) => {
            Object::from([$(($key.to_string(), $val)),*])
        };
    }

    fn s(v: &str) -> Value {
        Value::String(v.to_string())
    }

    fn o(obj: Object) -> Value {
        Value::Object(obj)
    }

    #[test]
    fn test_parse_ndmc_output() {
        assert_eq!(
            parse_output(NDMC_OUTPUT).unwrap(),
            vec![
                obj! {
                    "host" => o(obj! {
                        "mac"        => s("dc:03:98:81:d1:04"),
                        "ip"         => s("192.168.2.18"),
                        "hostname"   => s("LGwebOSTV"),
                        "name"       => s("LGwebOSTV"),
                        "registered" => s("yes"),
                        "active"     => s("yes"),
                        "rxbytes"    => s("280232502"),
                        "txbytes"    => s("4729253"),
                        "uptime"     => s("20335"),
                        "first-seen" => s("20330"),
                        "last-seen"  => s("15"),
                        "interface"  => o(obj! {
                            "id"          => s("Bridge0"),
                            "name"        => s("Home"),
                            "description" => s("Home network"),
                        }),
                        "dhcp" => o(obj! {
                            "expires" => s("4866"),
                        }),
                        "mws" => o(obj! {
                            "cid"  => s("e0b91322-8299-11ea-8f23-9317f81e1607"),
                            "ap"   => s("WifiMaster1/AccessPoint0"),
                            "psm"  => s("no"),
                            "rssi" => s("-41"),
                        }),
                    }),
                },
                obj! {
                    "host" => o(obj! {
                        "mac"        => s("70:89:76:dc:16:53"),
                        "ip"         => s("192.168.2.89"),
                        "hostname"   => s(""),
                        "name"       => s(""),
                        "registered" => s("no"),
                        "active"     => s("yes"),
                        "uptime"     => s("35622"),
                        "interface"  => o(obj! {
                            "id"   => s("Bridge0"),
                            "name" => s("Home"),
                        }),
                    }),
                },
            ],
        );
    }

    #[test]
    fn test_empty_output() {
        assert_eq!(parse_output("").unwrap(), vec![]);
        assert_eq!(parse_output("\n\n").unwrap(), vec![]);
    }
}
