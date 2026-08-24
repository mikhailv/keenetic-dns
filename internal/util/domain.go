package util

import "strings"

func FQDN(domain string) string {
	if domain == "" || strings.HasSuffix(domain, ".") {
		return domain
	}
	return domain + "."
}

func TrimFQDN(domain string) string {
	return strings.TrimSuffix(domain, ".")
}
