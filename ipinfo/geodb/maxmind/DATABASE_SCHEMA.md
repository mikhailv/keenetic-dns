# MaxMind GeoLite2 Database Schema

Verified by iterating all records in actual `.mmdb` files using `go run inspect.go schema`.

## 1. GeoLite2-ASN (1,079,993 records)

Flat structure, 2 fields. Both present in every record.

| Field | Type | Count | Example |
|---|---|---|---|
| `autonomous_system_number` | uint64 | 1,079,993 | 13335 |
| `autonomous_system_organization` | string | 1,079,993 | "Cloudflare, Inc." |

## 2. GeoLite2-Country (1,217,727 records)

| Field | Type | Count | Example |
|---|---|---|---|
| `continent.code` | string | 1,216,569 | "AS" |
| `continent.geoname_id` | uint64 | 1,216,569 | 6255147 |
| `continent.names.{de,en,es,fr,ja,pt-BR,ru,zh-CN}` | string | 1,216,569 | "Asia" |
| `country.iso_code` | string | 1,215,821 | "CN" |
| `country.geoname_id` | uint64 | 1,215,821 | 1814991 |
| `country.is_in_european_union` | bool | 354,070 | true |
| `country.names.{de,en,es,fr,ja,pt-BR,ru,zh-CN}` | string | 1,215,821 | "China" |
| `registered_country.iso_code` | string | 1,216,516 | "AU" |
| `registered_country.geoname_id` | uint64 | 1,216,516 | 2077456 |
| `registered_country.is_in_european_union` | bool | 275,217 | true |
| `registered_country.names.{de,en,es,fr,ja,pt-BR,ru,zh-CN}` | string | 1,216,516 | "Australia" |
| `represented_country.iso_code` | string | 33 | "US" |
| `represented_country.geoname_id` | uint64 | 33 | 6252001 |
| `represented_country.type` | string | 33 | "military" |
| `represented_country.names.{de,en,es,fr,ja,pt-BR,ru,zh-CN}` | string | 33 | "United States" |

- `country` = where the IP is geolocated
- `registered_country` = where the IP block is registered (often the same)
- `represented_country` = rare (only 33 records), for special cases like military bases

## 3. GeoLite2-City (5,722,033 records)

Superset of Country, adds `city`, `subdivisions`, `postal`, and `location`.

| Field | Type | Count | Example |
|---|---|---|---|
| `continent.code` | string | 5,720,875 | "AS" |
| `continent.geoname_id` | uint64 | 5,720,875 | 6255147 |
| `continent.names.{...}` | string | 5,720,875 | "Asia" |
| `country.iso_code` | string | 5,720,127 | "CN" |
| `country.geoname_id` | uint64 | 5,720,127 | 1814991 |
| `country.is_in_european_union` | bool | 1,251,701 | true |
| `country.names.{...}` | string | 5,720,127 | "China" |
| `registered_country.iso_code` | string | 5,720,546 | "AU" |
| `registered_country.geoname_id` | uint64 | 5,720,546 | 2077456 |
| `registered_country.is_in_european_union` | bool | 1,303,423 | true |
| `registered_country.names.{...}` | string | 5,720,546 | "Australia" |
| `represented_country.iso_code` | string | 33 | "US" |
| `represented_country.geoname_id` | uint64 | 33 | 6252001 |
| `represented_country.type` | string | 33 | "military" |
| `represented_country.names.{...}` | string | 33 | "United States" |
| **`city.geoname_id`** | uint64 | 4,549,726 | 1862415 |
| **`city.names.{...}`** | string | 2,486,075–4,549,726 | "Hiroshima" |
| **`subdivisions`** | array | 4,627,472 | [...] |
| **`subdivisions.[].iso_code`** | string | 5,034,425 | "34" |
| **`subdivisions.[].geoname_id`** | uint64 | 5,034,425 | 1862413 |
| **`subdivisions.[].names.{...}`** | string | 3,906,737–5,034,425 | "Hiroshima" |
| **`postal.code`** | string | 4,402,103 | "730-0851" |
| **`location.latitude`** | float64 | 5,720,875 | 34.7732 |
| **`location.longitude`** | float64 | 5,720,875 | 113.722 |
| **`location.accuracy_radius`** | uint64 | 5,720,875 | 1000 |
| **`location.time_zone`** | string | 5,720,875 | "Asia/Shanghai" |
| **`location.metro_code`** | uint64 | 2,648,755 | 810 |

`{...}` = localized names in 8 locales: `de`, `en`, `es`, `fr`, `ja`, `pt-BR`, `ru`, `zh-CN`.
Not all locales are present for every record (e.g. `city.names.en` has 4.5M entries but `city.names.es` only 2.5M).

## Notes

- The `network` (CIDR prefix) is not stored as a record field — it's returned by the lookup/iteration API as metadata.
- Not all fields are populated for every IP. For example, `1.1.1.1` only has `registered_country` with no `continent` or `country`.
- `is_in_european_union` is only present when `true`.
- `represented_country` exists in only 33 records (all type "military").
- `subdivisions` is an array ordered largest to smallest (e.g. England → Bromley).
- `metro_code` is deprecated (US-only Google ad-targeting code), present in ~2.6M records.
