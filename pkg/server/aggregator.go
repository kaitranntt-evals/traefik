package server

import (
	"context"
	"slices"
	"strings"

	"github.com/go-acme/lego/v5/challenge/tlsalpn01"
	"github.com/rs/zerolog/log"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/muxer"
	httpmuxer "github.com/traefik/traefik/v3/pkg/muxer/http"
	"github.com/traefik/traefik/v3/pkg/observability/logs"
	otypes "github.com/traefik/traefik/v3/pkg/observability/types"
	"github.com/traefik/traefik/v3/pkg/server/provider"
	traefiktls "github.com/traefik/traefik/v3/pkg/tls"
)

func mergeConfiguration(configurations dynamic.Configurations, defaultEntryPoints []string) dynamic.Configuration {
	// TODO: see if we can use DeepCopies inside, so that the given argument is left
	// untouched, and the modified copy is returned.
	conf := dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{
			Routers:           make(map[string]*dynamic.Router),
			Middlewares:       make(map[string]*dynamic.Middleware),
			Services:          make(map[string]*dynamic.Service),
			Models:            make(map[string]*dynamic.Model),
			ServersTransports: make(map[string]*dynamic.ServersTransport),
		},
		TCP: &dynamic.TCPConfiguration{
			Routers:           make(map[string]*dynamic.TCPRouter),
			Services:          make(map[string]*dynamic.TCPService),
			Middlewares:       make(map[string]*dynamic.TCPMiddleware),
			Models:            make(map[string]*dynamic.TCPModel),
			ServersTransports: make(map[string]*dynamic.TCPServersTransport),
		},
		UDP: &dynamic.UDPConfiguration{
			Routers:  make(map[string]*dynamic.UDPRouter),
			Services: make(map[string]*dynamic.UDPService),
		},
		TLS: &dynamic.TLSConfiguration{
			Stores:  make(map[string]traefiktls.Store),
			Options: make(map[string]traefiktls.Options),
		},
	}

	var defaultTLSOptionProviders []string
	var defaultTLSStoreProviders []string
	for pvd, configuration := range configurations {
		if configuration.HTTP != nil {
			for routerName, router := range configuration.HTTP.Routers {
				// If no entrypoint is defined, and the router has no parentRefs (i.e. is not a child router),
				// we set the default entrypoints.
				if len(router.EntryPoints) == 0 && router.ParentRefs == nil {
					log.Debug().
						Str(logs.RouterName, routerName).
						Strs(logs.EntryPointName, defaultEntryPoints).
						Msg("No entryPoint defined for this router, using the default one(s) instead")
					router.EntryPoints = slices.Clone(defaultEntryPoints)
				}

				// The `ruleSyntax` option is deprecated.
				// We exclude the "default" value to avoid logging it,
				// as it is the value used for internal models and computed rules.
				if router.RuleSyntax != "" && router.RuleSyntax != "default" {
					log.Warn().
						Str(logs.RouterName, routerName).
						Msg("Router's `ruleSyntax` option is deprecated, please remove any usage of this option.")
				}

				var qualifiedParentRefs []string
				for _, parentRef := range router.ParentRefs {
					if parts := strings.Split(parentRef, "@"); len(parts) == 1 {
						parentRef = provider.MakeQualifiedName(pvd, parentRef)
					}

					qualifiedParentRefs = append(qualifiedParentRefs, parentRef)
				}
				router.ParentRefs = qualifiedParentRefs

				conf.HTTP.Routers[provider.MakeQualifiedName(pvd, routerName)] = router
			}
			for middlewareName, middleware := range configuration.HTTP.Middlewares {
				conf.HTTP.Middlewares[provider.MakeQualifiedName(pvd, middlewareName)] = middleware
			}
			for serviceName, service := range configuration.HTTP.Services {
				conf.HTTP.Services[provider.MakeQualifiedName(pvd, serviceName)] = service
			}
			for modelName, model := range configuration.HTTP.Models {
				conf.HTTP.Models[provider.MakeQualifiedName(pvd, modelName)] = model
			}
			for serversTransportName, serversTransport := range configuration.HTTP.ServersTransports {
				conf.HTTP.ServersTransports[provider.MakeQualifiedName(pvd, serversTransportName)] = serversTransport
			}
		}

		if configuration.TCP != nil {
			for routerName, router := range configuration.TCP.Routers {
				if len(router.EntryPoints) == 0 {
					log.Debug().
						Str(logs.RouterName, routerName).
						Msgf("No entryPoint defined for this TCP router, using the default one(s) instead: %+v", defaultEntryPoints)
					router.EntryPoints = slices.Clone(defaultEntryPoints)
				}
				conf.TCP.Routers[provider.MakeQualifiedName(pvd, routerName)] = router
			}
			for middlewareName, middleware := range configuration.TCP.Middlewares {
				conf.TCP.Middlewares[provider.MakeQualifiedName(pvd, middlewareName)] = middleware
			}
			for serviceName, service := range configuration.TCP.Services {
				conf.TCP.Services[provider.MakeQualifiedName(pvd, serviceName)] = service
			}
			for modelName, model := range configuration.TCP.Models {
				conf.TCP.Models[provider.MakeQualifiedName(pvd, modelName)] = model
			}
			for serversTransportName, serversTransport := range configuration.TCP.ServersTransports {
				conf.TCP.ServersTransports[provider.MakeQualifiedName(pvd, serversTransportName)] = serversTransport
			}
		}

		if configuration.UDP != nil {
			for routerName, router := range configuration.UDP.Routers {
				conf.UDP.Routers[provider.MakeQualifiedName(pvd, routerName)] = router
			}
			for serviceName, service := range configuration.UDP.Services {
				conf.UDP.Services[provider.MakeQualifiedName(pvd, serviceName)] = service
			}
		}

		if configuration.TLS != nil {
			for _, cert := range configuration.TLS.Certificates {
				if slices.Contains(cert.Stores, tlsalpn01.ACMETLS1Protocol) && pvd != "tlsalpn.acme" {
					continue
				}

				conf.TLS.Certificates = append(conf.TLS.Certificates, cert)
			}

			for key, store := range configuration.TLS.Stores {
				if key != traefiktls.DefaultTLSStoreName {
					key = provider.MakeQualifiedName(pvd, key)
				} else {
					defaultTLSStoreProviders = append(defaultTLSStoreProviders, pvd)
				}
				conf.TLS.Stores[key] = store
			}

			for tlsOptionsName, options := range configuration.TLS.Options {
				if tlsOptionsName != "default" {
					tlsOptionsName = provider.MakeQualifiedName(pvd, tlsOptionsName)
				} else {
					defaultTLSOptionProviders = append(defaultTLSOptionProviders, pvd)
				}

				conf.TLS.Options[tlsOptionsName] = options
			}
		}
	}

	if len(defaultTLSStoreProviders) > 1 {
		log.Error().Msgf("Default TLS Store defined in multiple providers: %v", defaultTLSStoreProviders)
		delete(conf.TLS.Stores, traefiktls.DefaultTLSStoreName)
	}

	if len(defaultTLSOptionProviders) == 0 {
		conf.TLS.Options[traefiktls.DefaultTLSConfigName] = traefiktls.DefaultTLSOptions
	} else if len(defaultTLSOptionProviders) > 1 {
		log.Error().Msgf("Default TLS Options defined in multiple providers %v", defaultTLSOptionProviders)
		// We do not set an empty tls.TLS{} as above so that we actually get a "cascading failure" later on,
		// i.e. routers depending on this missing TLS option will fail to initialize as well.
		delete(conf.TLS.Options, traefiktls.DefaultTLSConfigName)
	}

	return conf
}

// resolveHTTPTLSOptions resolves the TLS options for the given routers, on a per
// entryPoint basis.
//
// TLS options conflicts (i.e. the same host served with different TLS options) can
// only be detected and arbitrated within a single TLS listener, that is to say within
// a single entryPoint. To honor that, routers are grouped per entryPoint and the
// conflict detection is run independently for each entryPoint.
//
// A router keeps its original name, and its resolved TLS options, for the entryPoints
// on which it does not conflict. For each entryPoint on which it conflicts, that
// entryPoint is removed from the router and a dedicated copy is emitted, named
// following the "ep-conflicted-name@provider" pattern.
//
// The conflict on that copy is arbitrated by falling back to the default TLS options,
// unless strictTLSOptions is enabled, in which case the copy is flagged as conflicting,
// which disables it.
func resolveHTTPTLSOptions(routers map[string]*dynamic.Router, strictTLSOptions bool) map[string]*dynamic.Router {
	if len(routers) == 0 {
		return routers
	}

	newRouters := make(map[string]*dynamic.Router)

	// Split every router per entryPoint.
	// Routers always have at least one entryPoint at this stage, as they are
	// defaulted in mergeConfiguration before applyModel and this resolution run.
	routersByEntryPoint := map[string]map[string]*dynamic.Router{}
	for name, router := range routers {
		if router.TLS == nil {
			newRouters[name] = router
			continue
		}

		router.TLS.ResolvedOptions = traefiktls.DefaultTLSConfigName
		if len(router.TLS.Options) > 0 && router.TLS.Options != traefiktls.DefaultTLSConfigName {
			router.TLS.ResolvedOptions = provider.GetQualifiedName(provider.AddInContext(context.Background(), name), router.TLS.Options)
		}

		for _, ep := range router.EntryPoints {
			if routersByEntryPoint[ep] == nil {
				routersByEntryPoint[ep] = map[string]*dynamic.Router{}
			}

			routersByEntryPoint[ep][name] = router
		}
	}

	// Resolve the TLS options independently for each entryPoint.
	conflictingRouters := make(map[string][]string, len(routersByEntryPoint))
	for ep, epRouters := range routersByEntryPoint {
		childDomains := resolveParentChildDomains(epRouters, routers)

		conflictingRouters[ep] = findConflictingRouters(ep, epRouters, childDomains, strictTLSOptions)
	}

	// A router conflicting on an entry point is copied under a new name, so the
	// child routes referencing it as a parent need that copy to keep resolving.
	conflictedNames := map[string][]string{}

	for name, router := range routers {
		router.EntryPoints = slices.DeleteFunc(router.EntryPoints, func(ep string) bool {
			deleted := slices.Contains(conflictingRouters[ep], name)
			if deleted {
				rt := router.DeepCopy()
				if strictTLSOptions {
					rt.TLS.ConflictingOptions = true
				} else {
					rt.TLS.ResolvedOptions = traefiktls.DefaultTLSConfigName
				}
				rt.EntryPoints = []string{ep}
				// The new name is not collision free but has very small possibility to collide.
				// TODO: rework this naming whenever we'll introduce a resource reference mechanism not based on a string.
				newName := ep + "-conflicted-" + name
				newRouters[newName] = rt
				conflictedNames[name] = append(conflictedNames[name], newName)
			}

			return deleted
		})

		if len(router.EntryPoints) > 0 {
			newRouters[name] = router
		}
	}

	for _, names := range conflictedNames {
		slices.Sort(names)
	}

	// A child router carries no entry point of its own, so it references every
	// renamed copy of its parent, and the original name only while it exists.
	for _, router := range newRouters {
		if len(router.ParentRefs) == 0 {
			continue
		}

		var parentRefs []string
		for _, parentRef := range router.ParentRefs {
			found := false

			if _, ok := newRouters[parentRef]; ok && !slices.Contains(parentRefs, parentRef) {
				parentRefs = append(parentRefs, parentRef)
				found = true
			}

			for _, newName := range conflictedNames[parentRef] {
				if !slices.Contains(parentRefs, newName) {
					parentRefs = append(parentRefs, newName)
				}
				found = true
			}

			// A reference to a parent that exists neither under its own name nor
			// under a renamed copy is kept as is, so the router still reports it.
			if !found && !slices.Contains(parentRefs, parentRef) {
				parentRefs = append(parentRefs, parentRef)
			}
		}

		router.ParentRefs = parentRefs
	}

	return newRouters
}

// resolveParentChildDomains determines which concrete hostnames a parent router effectively
// serves through its child routers on an entryPoint and which overlapping host expressions it
// shares with other providers' routers when at least one router is a parent.
func resolveParentChildDomains(epRouters map[string]*dynamic.Router, allRouters map[string]*dynamic.Router) map[string][]string {
	providerOf := func(name string) string { _, sfx, _ := strings.Cut(name, "@"); return sfx }
	isBroad := func(d string) bool { return d == "*" || strings.HasPrefix(d, "*.") || strings.HasPrefix(d, "**.") }

	coverRank := func(patterns []string, d string) int {
		if len(patterns) == 0 {
			return 0
		}
		best := -1
		for _, p := range patterns {
			rank := -1
			switch {
			case p == "*":
				rank = 0
			case strings.EqualFold(p, d):
				rank = 10000 + len(p)
			case strings.HasPrefix(p, "**.") && muxer.DomainMatchHostExpression(d, p):
				rank = 1 + len(strings.TrimPrefix(p, "**."))
			case strings.HasPrefix(p, "*.") && !strings.HasPrefix(d, "**.") && muxer.DomainMatchHostExpression(d, p):
				rank = 1 + len(strings.TrimPrefix(p, "*."))
			}
			if rank > best {
				best = rank
			}
		}
		return best
	}

	rootOwn := map[string][]string{}
	for name, router := range epRouters {
		if router.TLS != nil {
			if own, err := httpmuxer.ParseDomains(router.Rule); err == nil {
				rootOwn[name] = own
			}
		}
	}

	parentDomains := map[string][]string{}
	childRules := map[string][][]string{}
	for _, router := range allRouters {
		if len(router.ParentRefs) == 0 {
			continue
		}
		domains, err := httpmuxer.ParseDomains(router.Rule)
		if err != nil {
			continue
		}
		for _, parentRef := range router.ParentRefs {
			if own, ok := rootOwn[parentRef]; ok {
				parentDomains[parentRef] = own
				childRules[parentRef] = append(childRules[parentRef], domains)
			}
		}
	}

	parentClaimsDomain := func(parentRef, d string) bool {
		rank := coverRank(parentDomains[parentRef], d)
		if rank < 0 {
			return false
		}
		pvd := providerOf(parentRef)
		for other, otherPatterns := range parentDomains {
			if other != parentRef && providerOf(other) == pvd && coverRank(otherPatterns, d) > rank {
				return false
			}
		}
		return true
	}

	childDomains := map[string][]string{}
	for parentRef, rules := range childRules {
		for _, domains := range rules {
			for _, c := range domains {
				if !isBroad(c) && parentClaimsDomain(parentRef, c) {
					childDomains[parentRef] = append(childDomains[parentRef], c)
				}
			}
		}
	}

	serves := func(name string, own []string, d string) bool {
		rules, isParent := childRules[name]
		if !isParent {
			return len(own) > 0 && coverRank(own, d) >= 0
		}
		return parentClaimsDomain(name, d) && slices.ContainsFunc(rules, func(child []string) bool {
			return (len(child) > 0 || len(own) > 0) && coverRank(child, d) >= 0
		})
	}

	rootAll := map[string][]string{}
	for name, own := range rootOwn {
		rootAll[name] = slices.Concat(own, childDomains[name])
	}
	for name, own := range rootOwn {
		_, isParent := childRules[name]
		for other, otherDomains := range rootAll {
			_, otherIsParent := childRules[other]
			if providerOf(name) == providerOf(other) || (!isParent && !otherIsParent) {
				continue
			}
			for _, d := range otherDomains {
				if serves(name, own, d) {
					childDomains[name] = append(childDomains[name], d)
				}
			}
		}
	}
	return childDomains
}

// findConflictingRouters returns the names of the routers, among the given
// single-entryPoint routers, that serve a host (SNI) also served by another router
// with a different resolved TLS option. Such routers are arbitrated by falling back
// to the default TLS options, or, when strictTLSOptions is enabled, by disabling them.
func findConflictingRouters(ep string, routers map[string]*dynamic.Router, childDomains map[string][]string, strictTLSOptions bool) []string {
	var conflicting []string
	effectiveOptions := map[string]string{}
	routerDomains := map[string][]string{}

	for name, router := range routers {
		if router.TLS == nil {
			continue
		}
		domains, err := httpmuxer.ParseDomains(router.Rule)
		if err != nil {
			continue
		}

		option := router.TLS.ResolvedOptions
		// The configured TLSOptions on a router without a domain in its rule cannot be selected when evaluating the SNI,
		// so if it is not the default one, it is a conflict.
		if len(domains) == 0 && option != traefiktls.DefaultTLSConfigName {
			conflicting = append(conflicting, name)
			if strictTLSOptions {
				log.Error().Msgf("On EntryPoint %q, router %q, with no domains in its rule, is configured with a TLS options different than the default one, the fallback to the default TLS options being disabled, it will be disabled", ep, name)
				continue
			}
			log.Error().Msgf("On EntryPoint %q, router %q, with no domains in its rule, is configured with a TLS options different than the default one, default TLSOptions will be applied for it", ep, name)
			option = traefiktls.DefaultTLSConfigName
		}

		effectiveOptions[name] = option
		routerDomains[name] = slices.Compact(slices.Sorted(slices.Values(append(domains, childDomains[name]...))))
	}

	for {
		routersByHostAndOption := map[string]map[string][]string{}
		for name, domains := range routerDomains {
			opt := effectiveOptions[name]
			for _, domain := range domains {
				if routersByHostAndOption[domain] == nil {
					routersByHostAndOption[domain] = map[string][]string{}
				}
				routersByHostAndOption[domain][opt] = append(routersByHostAndOption[domain][opt], name)
			}
		}

		changed := false
		for domain, routersByOption := range routersByHostAndOption {
			if len(routersByOption) == 1 {
				continue
			}
			var routersInConflict []string
			for _, names := range routersByOption {
				for _, name := range names {
					if !slices.Contains(conflicting, name) {
						conflicting = append(conflicting, name)
					}
					if !strictTLSOptions && effectiveOptions[name] != traefiktls.DefaultTLSConfigName {
						effectiveOptions[name] = traefiktls.DefaultTLSConfigName
						changed = true
					}
				}
				routersInConflict = append(routersInConflict, names...)
			}
			if strictTLSOptions {
				log.Error().Msgf("On EntryPoint %q, Host %q is served by multiple routers with different TLS options, the fallback to the default TLS options being disabled, the following routers are disabled: %v", ep, domain, routersInConflict)
			} else {
				log.Error().Msgf("On EntryPoint %q, Host %q is served by multiple routers with different TLS options, default TLSOptions will be applied for the following routers: %v", ep, domain, routersInConflict)
			}
		}

		if strictTLSOptions || !changed {
			break
		}
	}

	return conflicting
}

func applyModel(cfg dynamic.Configuration) dynamic.Configuration {
	if cfg.HTTP != nil && len(cfg.HTTP.Models) > 0 {
		rts := make(map[string]*dynamic.Router)

		modelRouterNames := make(map[string][]string)
		for name, rt := range cfg.HTTP.Routers {
			// Only root routers can have models applied.
			if rt.ParentRefs != nil {
				rts[name] = rt
				continue
			}

			router := rt.DeepCopy()

			if !router.DefaultRule && router.RuleSyntax == "" {
				for modelName, model := range cfg.HTTP.Models {
					// models cannot be provided by another provider than the internal one.
					if !strings.HasSuffix(modelName, "@internal") {
						continue
					}
					router.RuleSyntax = model.DefaultRuleSyntax
					break
				}
			}

			eps := router.EntryPoints
			router.EntryPoints = nil

			for _, epName := range eps {
				m, ok := cfg.HTTP.Models[epName+"@internal"]
				if ok {
					cp := router.DeepCopy()

					cp.EntryPoints = []string{epName}

					if cp.TLS == nil {
						cp.TLS = m.TLS
					}

					cp.Middlewares = append(m.Middlewares, cp.Middlewares...)

					if m.DeniedEncodedPathCharacters != nil {
						// As the denied encoded path characters option is not configurable at the router level,
						// we can simply copy the whole structure to override the router's default config.
						cp.DeniedEncodedPathCharacters = m.DeniedEncodedPathCharacters
					}

					if cp.Observability == nil {
						cp.Observability = &dynamic.RouterObservabilityConfig{}
					}

					if cp.Observability.AccessLogs == nil {
						cp.Observability.AccessLogs = m.Observability.AccessLogs
					}

					if cp.Observability.Metrics == nil {
						cp.Observability.Metrics = m.Observability.Metrics
					}

					if cp.Observability.Tracing == nil {
						cp.Observability.Tracing = m.Observability.Tracing
					}

					if cp.Observability.TraceVerbosity == "" {
						cp.Observability.TraceVerbosity = m.Observability.TraceVerbosity
					}

					rtName := name
					if len(eps) > 1 {
						rtName = epName + "-" + name
						modelRouterNames[name] = append(modelRouterNames[name], rtName)
					}

					rts[rtName] = cp
				} else {
					router.EntryPoints = append(router.EntryPoints, epName)

					rts[name] = router
				}
			}
		}

		for _, rt := range rts {
			if rt.ParentRefs == nil {
				continue
			}

			var parentRefs []string
			for _, ref := range rt.ParentRefs {
				// Only add the initial parent ref if it still exists.
				if _, ok := rts[ref]; ok {
					parentRefs = append(parentRefs, ref)
				}

				if names, ok := modelRouterNames[ref]; ok {
					parentRefs = append(parentRefs, names...)
				}
			}

			rt.ParentRefs = parentRefs
		}

		cfg.HTTP.Routers = rts
	}

	// Apply the default observability model to HTTP routers.
	applyDefaultObservabilityModel(cfg)

	if cfg.TCP == nil || len(cfg.TCP.Models) == 0 {
		return cfg
	}

	tcpRouters := make(map[string]*dynamic.TCPRouter)

	for name, rt := range cfg.TCP.Routers {
		router := rt.DeepCopy()

		if router.RuleSyntax == "" {
			for _, model := range cfg.TCP.Models {
				router.RuleSyntax = model.DefaultRuleSyntax
				break
			}
		}

		tcpRouters[name] = router
	}

	cfg.TCP.Routers = tcpRouters

	return cfg
}

// applyDefaultObservabilityModel applies the default observability model to the configuration.
// This function is used to ensure that the observability configuration is set for all routers,
// and make sure it is serialized and available in the API.
// We could have introduced a "default" model, but it would have been more complex to manage for now.
// This could be generalized in the future.
// TODO: check if we can remove this and rely on the SetDefaults instead.
func applyDefaultObservabilityModel(cfg dynamic.Configuration) {
	if cfg.HTTP != nil {
		for _, router := range cfg.HTTP.Routers {
			// Only root routers can have models applied.
			if router.ParentRefs != nil {
				continue
			}

			if router.Observability == nil {
				router.Observability = &dynamic.RouterObservabilityConfig{
					AccessLogs:     new(true),
					Metrics:        new(true),
					Tracing:        new(true),
					TraceVerbosity: otypes.MinimalVerbosity,
				}

				continue
			}

			if router.Observability.AccessLogs == nil {
				router.Observability.AccessLogs = new(true)
			}

			if router.Observability.Metrics == nil {
				router.Observability.Metrics = new(true)
			}

			if router.Observability.Tracing == nil {
				router.Observability.Tracing = new(true)
			}

			if router.Observability.TraceVerbosity == "" {
				router.Observability.TraceVerbosity = otypes.MinimalVerbosity
			}
		}
	}
}
