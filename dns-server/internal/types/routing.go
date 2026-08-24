package types

type RoutingAction string

const (
	ActionRouted  RoutingAction = "routed"
	ActionIgnored RoutingAction = "ignored"
)

type IPRouting struct {
	Action RoutingAction `json:"action"`
	Static bool          `json:"static"`
	Iface  string        `json:"iface,omitempty"`
	Reason string        `json:"reason"`
	Added  bool          `json:"added,omitempty"`
}

type IPRoutings map[IPv4]IPRouting

func (s IPRoutings) Has(action RoutingAction) bool {
	for _, it := range s {
		if it.Action == action {
			return true
		}
	}
	return false
}

func (s *IPRoutings) AddRoute(iface, reason string, ip IPv4) {
	s.add(ActionRouted, false, iface, reason, ip)
}

func (s *IPRoutings) AddStaticRoute(iface, reason string, ip IPv4) {
	s.add(ActionRouted, true, iface, reason, ip)
}

func (s *IPRoutings) AddIgnored(reason string, ip IPv4) {
	s.add(ActionIgnored, false, "", reason, ip)
}

func (s *IPRoutings) add(action RoutingAction, static bool, iface, reason string, ip IPv4) {
	if *s == nil {
		*s = IPRoutings{}
	}
	(*s)[ip] = IPRouting{
		Action: action,
		Static: static,
		Iface:  iface,
		Reason: reason,
	}
}
