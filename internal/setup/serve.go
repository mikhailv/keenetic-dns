package setup

import "context"

func Serve(ctx context.Context, server interface{ Serve(context.Context) error }) {
	ExitIfError(server.Serve(ctx))
}
