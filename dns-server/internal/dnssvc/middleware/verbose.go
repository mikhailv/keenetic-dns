package middleware

import (
	"context"
	"fmt"
	"strings"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
)

var _ dnssvc.Middleware = VerboseMiddleware

func VerboseMiddleware(handler dnssvc.Handler) dnssvc.Handler {
	return func(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
		fmt.Println(">> DNS:\n" + indentTextBlock(req.String(), "\t"))
		resp, err := handler(ctx, req)
		if err != nil {
			fmt.Println("<< DNS error: " + err.Error())
		} else {
			fmt.Println("<< DNS:\n" + indentTextBlock(resp.String(), "\t"))
		}
		return resp, err
	}
}

func indentTextBlock(text string, indentation string) string {
	return indentation + strings.Join(strings.Split(text, "\n"), "\n"+indentation)
}
