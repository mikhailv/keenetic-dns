package api

import "log/slog"

func (r *Rule) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("from", r.From),
		slog.Int("table", int(r.Table)),
		slog.Int("priority", int(r.Priority)),
	)
}

func (r *Route) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("table", int(r.Table)),
		slog.String("iface", r.Iface),
		slog.String("addr", r.Address),
	)
}
