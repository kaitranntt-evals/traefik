package gateway

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/provider"
	"k8s.io/utils/ptr"
	gatev1 "sigs.k8s.io/gateway-api/apis/v1"
)

func configureListenerIsolation(gateway *gatev1.Gateway, listeners []gatewayListener, conf *dynamic.Configuration) {
	for i, listener := range listeners {
		if !listener.Attached || (listener.Protocol != gatev1.HTTPProtocolType && listener.Protocol != gatev1.HTTPSProtocolType) {
			continue
		}

		listeners[i].ExcludedHostnames = excludedListenerHostnames(listener, listeners)
		if listener.Protocol != gatev1.HTTPSProtocolType {
			continue
		}

		if !slices.ContainsFunc(listeners, func(other gatewayListener) bool {
			return other.Attached && other.Name != listener.Name && other.Port == listener.Port && other.Protocol == listener.Protocol
		}) {
			continue
		}

		rule := `Host("*")`
		if listener.Hostname != nil {
			rule = buildListenerHostRule([]gatev1.Hostname{*listener.Hostname})
		}
		if excludedRule := buildListenerHostRule(listeners[i].ExcludedHostnames); excludedRule != "" {
			rule += " && !" + excludedRule
		}
		listeners[i].SNIHostRule = rule

		// Misdirected requests must also be rejected when no route matches the path.
		name := provider.Normalize(fmt.Sprintf("gateway-%s-%s-%s-fallback", gateway.Namespace, gateway.Name, listener.Name))
		conf.HTTP.Routers[name] = &dynamic.Router{
			EntryPoints: []string{listener.EPName},
			Rule:        rule,
			RuleSyntax:  "default",
			Priority:    -1,
			Service:     name,
			TLS:         &dynamic.RouterTLSConfig{SNIHostRule: rule},
		}
		conf.HTTP.Services[name] = &dynamic.Service{
			Weighted: &dynamic.WeightedRoundRobin{
				Services: []dynamic.WRRService{{Name: "no-matching-route", Status: new(http.StatusNotFound), Weight: new(1)}},
			},
		}
	}
}

func buildListenerHostRule(hostnames []gatev1.Hostname) string {
	var rules []string
	for _, hostname := range hostnames {
		host := string(hostname)
		// Gateway wildcards span multiple labels; Host also handles a trailing dot in the request authority.
		if strings.HasPrefix(host, "*.") {
			host = "*" + host
		}
		rules = append(rules, fmt.Sprintf("Host(%q)", host))
	}
	rule := strings.Join(rules, " || ")
	if len(rules) > 1 {
		return "(" + rule + ")"
	}
	return rule
}

func excludedListenerHostnames(listener gatewayListener, listeners []gatewayListener) []gatev1.Hostname {
	hostname := ptr.Deref(listener.Hostname, "")
	var excluded []gatev1.Hostname
	for _, other := range listeners {
		if !other.Attached || other.Port != listener.Port || other.Protocol != listener.Protocol {
			continue
		}

		otherHostname := ptr.Deref(other.Hostname, "")
		if otherHostname == "" || otherHostname == hostname {
			continue
		}

		if hostname == "" || findMatchingHostname(hostname, otherHostname) == otherHostname {
			excluded = append(excluded, otherHostname)
		}
	}

	slices.Sort(excluded)
	return slices.Compact(excluded)
}
