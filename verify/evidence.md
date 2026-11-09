Observations for audit run `v-2c904244` of `traefik-gateway-listener-isolation`. Every block below is command output captured in the audit VM; the full captures are under `verify/logs/`, the probes under `verify/repro/`. No production file of this branch is modified: probes are `//go:build verify` test files copied into a claim-branch worktree, and every script restores the worktree when it exits.

## C1

Claim: the authored listener-isolation tests do not demonstrate that an identified broader listener's backend route excludes traffic addressed to a more specific listener's hostname. Adjudicated separately on `evalon/traefik-ga-c8563c30` (HEAD `11f39190100d9c03b1546416f97d5b4e03655d88`), `evalon/traefik-ga-b52f8b88` (HEAD `a1248f57c7f42900adf4600f4703e0df8dfd3028`) and `evalon/traefik-ga-12fa8560` (HEAD `cfa9e2537c3104bc73b59df463de812673de2a71`).

Setup and the one command run per branch:

```sh
cd ~/repos/traefik
git remote add claims https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation   # same proxy host as origin in the audit VM
git fetch claims 'refs/heads/evalon/*:refs/remotes/claims/evalon/*'
git worktree add --detach ~/wt/c8563c30 claims/evalon/traefik-ga-c8563c30
git worktree add --detach ~/wt/b52f8b88 claims/evalon/traefik-ga-b52f8b88
git worktree add --detach ~/wt/12fa8560 claims/evalon/traefik-ga-12fa8560
unzip eval_tests.zip -d ~/eval        # byte-exact eval fixtures: ~/eval/tests/{eval_https_test.go,eval_isolation_test.go,export_eval_test.go}
go version                            # go1.26.3 linux/amd64
```

```sh
bash verify/repro/c1_run.sh ~/wt/c8563c30 > verify/logs/c1_c8563c30.txt 2>&1
bash verify/repro/c1_run.sh ~/wt/b52f8b88 > verify/logs/c1_b52f8b88.txt 2>&1
bash verify/repro/c1_run.sh ~/wt/12fa8560 > verify/logs/c1_12fa8560.txt 2>&1
```

`c1_run.sh` runs, in order: (1) the authored tests untouched (`go test -count=1 -v -run '^TestLoad(Routes|HTTPRoutes|GRPCRoutes)_(listenerIsolation|httpsListenerIsolation)$' ./pkg/provider/kubernetes/gateway`); (2) `verify/repro/c1_route_identity_probe_test.go` (`go test -tags verify -count=1 -v -run TestVerifyC1`), which loads the fixtures the authored tests load, through the same loader, and dispatches requests through `httpmuxer.Muxer` behind the production `requestdecorator`; (3) the authored tests again with one injected line, `verifyC1Hook(t, conf)`, right after `p.loadConfigurationFromGateways`, which exchanges rule, priority and TLS between the two router objects of the loaded configuration; (4-8) the authored tests with untouched test code after exchanging, in the fixture YAML, the parent binding (`parentRefs[].sectionName`) or the complete `spec.rules` block between the route attached to the broader listener and the route attached to a more specific one.

What the authored tests assert (inspection, each paired with the runs below). None of the three branches dispatches a request in its added tests (`git diff 35c7530ac0906b5407a3733de173d58d9c427949 HEAD -- '*_test.go' | grep -E '^\+' | grep -cE 'httpmuxer|ServeHTTP|httptest|NewMuxer|AddRoute|ParentRefs'` prints `0` on each). They compare an exact collection built from the loaded configuration:

- c8563c30, `TestLoadRoutes_listenerIsolation`: `rules[router.Rule] = tlsOptions` then `assert.Equal(t, test.expectedRules, rules)` (kubernetes_test.go:10406).
- b52f8b88, `TestLoadRoutes_listenerIsolation`: `routerSummary{Rule, Priority, TLS}` then `assert.ElementsMatch(t, test.wantRouters, routers)` (kubernetes_test.go:9799).
- 12fa8560, `TestLoadHTTPRoutes_listenerIsolation`: `assert.ElementsMatch(t, wantRules, slices.Collect(maps.Keys(routers)))` (kubernetes_test.go:10647).

In each HTTP fixture (`fixtures/httproute/with_listener_isolation.yml`) every HTTPRoute has a distinct path (`/empty-hostname` for the route attached to the listener without hostname, `/abc-foo-example-com` for the one attached to listener `abc.foo.example.com`, ...), and all routes use backend `whoami:80` (`grep -A1 'value: /' <fixture> | grep value | sort | uniq -c` prints each path once on each branch). That distinct path is what ties a rule in the asserted collection to one HTTPRoute, and through its `parentRefs[].sectionName` to one listener.

### C1 on evalon/traefik-ga-c8563c30

Authored tests, untouched:

```console
##### 1. authored tests, untouched
--- PASS: TestLoadRoutes_listenerIsolation (0.00s)
    --- PASS: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_HTTP_listeners_with_nested_hostnames (0.11s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTP_listeners_with_nested_hostnames (0.11s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners_with_distinct_hostnames (0.11s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.144s
```

Dispatch on the configuration the authored HTTP test loads (not exchanged). The router generated for the HTTPRoute attached to the broader listener is the one carrying the exclusion rule of the asserted collection, and a request for the more specific hostname with that route's own path is rejected:

```console
CONFIG   fixture=httproute/with_listener_isolation.yml rule-exchange-in-loaded-config=false
ROUTER   key=httproute-default-attached-to-abc-foo-example-com-gw-default-my-gateway-ep-web-0-35a1b138b6f3114c1053 service=httproute-default-attached-to-abc-foo-example-com-gw-default-my-gateway-ep-web-0-35a1b138b6f3114c1053-wrr priority=12020 rule=Host("abc.foo.example.com") && (Path("/abc-foo-example-com") || PathPrefix("/abc-foo-example-com/"))
ROUTER   key=httproute-default-attached-to-empty-hostname-gw-default-my-gateway-ep-web-0-5a94afa24869965c7e75 service=httproute-default-attached-to-empty-hostname-gw-default-my-gateway-ep-web-0-5a94afa24869965c7e75-wrr priority=11501 rule=!(Host("**.example.com") || Host("**.foo.example.com") || Host("abc.foo.example.com")) && (Path("/empty-hostname") || PathPrefix("/empty-hostname/"))
DISPATCH GET http://unrelated.test/empty-hostname -> 200 servedBy=BROADER-BACKEND-ROUTER(empty-hostname) rule=!(Host("**.example.com") || Host("**.foo.example.com") || Host("abc.foo.example.com")) && (Path("/empty-hostname") || PathPrefix("/empty-hostname/"))
DISPATCH GET http://abc.foo.example.com/empty-hostname -> 404 servedBy=<no router> rule=
DISPATCH GET http://abc.foo.example.com/abc-foo-example-com -> 200 servedBy=SPECIFIC-BACKEND-ROUTER(abc-foo-example-com) rule=Host("abc.foo.example.com") && (Path("/abc-foo-example-com") || PathPrefix("/abc-foo-example-com/"))
DISPATCH GET http://unrelated.test/abc-foo-example-com -> 404 servedBy=<no router> rule=
```

In-memory exchange of rule/priority/TLS between the two router objects (router keys and services left in place) — the authored tests stay green, i.e. they do not key a rule to a router name or service:

```console
##### 3. authored tests after exchanging rule/priority/TLS between the two router objects in the loaded configuration (hook injected after loadConfigurationFromGateways)
HOOK-INJECTED into 1 authored test bodies
    kubernetes_test.go:10397: VERIFY hook: exchanged rule/priority/TLS between the backend routers of listeners "empty-hostname" and "wildcard-example-com"
    kubernetes_test.go:10397: VERIFY hook: exchanged rule/priority/TLS between the backend routers of listeners "empty-hostname" and "abc-foo-example-com"
--- PASS: TestLoadRoutes_listenerIsolation (0.00s)
    --- PASS: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_HTTP_listeners_with_nested_hostnames (0.14s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTP_listeners_with_nested_hostnames (0.14s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners_with_distinct_hostnames (0.14s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.173s
```

Dispatch on that exchanged configuration does tell the two apart (the broader path is now served by the router of the more specific route), and still rejects the more specific hostname on the broader path:

```console
CONFIG   fixture=httproute/with_listener_isolation.yml rule-exchange-in-loaded-config=true
DISPATCH GET http://unrelated.test/empty-hostname -> 200 servedBy=SPECIFIC-BACKEND-ROUTER(abc-foo-example-com) rule=!(Host("**.example.com") || Host("**.foo.example.com") || Host("abc.foo.example.com")) && (Path("/empty-hostname") || PathPrefix("/empty-hostname/"))
DISPATCH GET http://abc.foo.example.com/empty-hostname -> 404 servedBy=<no router> rule=
DISPATCH GET http://abc.foo.example.com/abc-foo-example-com -> 200 servedBy=BROADER-BACKEND-ROUTER(empty-hostname) rule=Host("abc.foo.example.com") && (Path("/abc-foo-example-com") || PathPrefix("/abc-foo-example-com/"))
DISPATCH GET http://unrelated.test/abc-foo-example-com -> 404 servedBy=<no router> rule=
```

Fixture exchange of the parent bindings between the broader route and the most specific route, authored test code untouched — the exact rule collection fails, naming the exclusion that moved to the other route:

```console
##### 4. authored tests, untouched test code, after a fixture exchange
FIXTURE-EXCHANGE pkg/provider/kubernetes/gateway/fixtures/httproute/with_listener_isolation.yml: parents between route attached-to-empty-hostname (listener empty-hostname) and route attached-to-abc-foo-example-com (listener abc-foo-example-com)
        	Error Trace:	/home/ubuntu/wt/c8563c30/pkg/provider/kubernetes/gateway/kubernetes_test.go:10406
        	Error:      	Not equal: 
        	            	- (string) (len=149) "!(Host(\"**.example.com\") || Host(\"**.foo.example.com\") || Host(\"abc.foo.example.com\")) && (Path(\"/empty-hostname\") || PathPrefix(\"/empty-hostname/\"))": (string) "",
        	            	+ (string) (len=159) "!(Host(\"**.example.com\") || Host(\"**.foo.example.com\") || Host(\"abc.foo.example.com\")) && (Path(\"/abc-foo-example-com\") || PathPrefix(\"/abc-foo-example-com/\"))": (string) "",
        	            	- (string) (len=100) "Host(\"abc.foo.example.com\") && (Path(\"/abc-foo-example-com\") || PathPrefix(\"/abc-foo-example-com/\"))": (string) "",
        	            	+ (string) (len=90) "Host(\"abc.foo.example.com\") && (Path(\"/empty-hostname\") || PathPrefix(\"/empty-hostname/\"))": (string) "",
--- FAIL: TestLoadRoutes_listenerIsolation (0.00s)
    --- PASS: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_HTTP_listeners_with_nested_hostnames (0.10s)
    --- FAIL: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTP_listeners_with_nested_hostnames (0.11s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners_with_distinct_hostnames (0.12s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.171s
```

Remaining exchanges (`spec.rules` blocks on the HTTP fixture; parent bindings between the broader route and the `*.example.com` route; gRPC fixture; HTTPS fixture):

```console
##### 5. authored tests, untouched test code, after a fixture exchange
FIXTURE-EXCHANGE pkg/provider/kubernetes/gateway/fixtures/httproute/with_listener_isolation.yml: rules between route attached-to-empty-hostname (listener empty-hostname) and route attached-to-abc-foo-example-com (listener abc-foo-example-com)
        	Error Trace:	/home/ubuntu/wt/c8563c30/pkg/provider/kubernetes/gateway/kubernetes_test.go:10406
--- FAIL: TestLoadRoutes_listenerIsolation (0.00s)
    --- PASS: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_HTTP_listeners_with_nested_hostnames (0.11s)
    --- FAIL: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTP_listeners_with_nested_hostnames (0.16s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners_with_distinct_hostnames (0.17s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.237s
##### 6. authored tests, untouched test code, after a fixture exchange
FIXTURE-EXCHANGE pkg/provider/kubernetes/gateway/fixtures/httproute/with_listener_isolation.yml: parents between route attached-to-empty-hostname (listener empty-hostname) and route attached-to-wildcard-example-com (listener wildcard-example-com)
        	Error Trace:	/home/ubuntu/wt/c8563c30/pkg/provider/kubernetes/gateway/kubernetes_test.go:10406
--- FAIL: TestLoadRoutes_listenerIsolation (0.00s)
    --- PASS: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_HTTP_listeners_with_nested_hostnames (0.20s)
    --- FAIL: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTP_listeners_with_nested_hostnames (0.21s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners_with_distinct_hostnames (0.21s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.289s
##### 7. authored tests, untouched test code, after a fixture exchange
FIXTURE-EXCHANGE pkg/provider/kubernetes/gateway/fixtures/grpcroute/with_listener_isolation.yml: parents between route attached-to-empty-hostname (listener empty-hostname) and route attached-to-wildcard-example-com (listener wildcard-example-com)
--- PASS: TestLoadRoutes_listenerIsolation (0.00s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners_with_distinct_hostnames (0.20s)
    --- PASS: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_HTTP_listeners_with_nested_hostnames (0.20s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTP_listeners_with_nested_hostnames (0.20s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.274s
##### 8. authored tests, untouched test code, after a fixture exchange
FIXTURE-EXCHANGE pkg/provider/kubernetes/gateway/fixtures/httproute/with_https_listeners_misdirected_requests.yml: parents between route http-app-3 (listener https-with-wildcard-hostname) and route http-app-4 (listener https-with-hostname-matching-wildcard)
--- PASS: TestLoadRoutes_listenerIsolation (0.00s)
    --- PASS: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_HTTP_listeners_with_nested_hostnames (0.11s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTP_listeners_with_nested_hostnames (0.12s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners_with_distinct_hostnames (0.12s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.151s
##### worktree after restore: 0 modified files
```

### C1 on evalon/traefik-ga-b52f8b88

Authored tests, untouched:

```console
##### 1. authored tests, untouched
--- PASS: TestLoadRoutes_listenerIsolation (0.00s)
    --- PASS: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_listeners_with_and_without_hostname (0.11s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoute_attached_to_every_listener_is_only_served_by_the_most_specific_one (0.10s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_with_hostnames_intersecting_the_listener_hostnames (0.11s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners_sharing_the_port_detect_the_misdirected_requests (0.11s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_listeners_with_hostnames_of_increasing_specificity (0.11s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.139s
```

Dispatch on the configuration the authored HTTP test loads (not exchanged). The router generated for the HTTPRoute attached to the broader listener is the one carrying the exclusion rule of the asserted collection, and a request for the more specific hostname with that route's own path is rejected:

```console
CONFIG   fixture=httproute/with_listener_isolation.yml rule-exchange-in-loaded-config=false
ROUTER   key=httproute-default-attaches-to-abc-foo-example-com-gw-default-my-gateway-ep-web-0-ee0d5da78ccdb047521a service=httproute-default-attaches-to-abc-foo-example-com-gw-default-my-gateway-ep-web-0-ee0d5da78ccdb047521a-wrr priority=12020 rule=Host("abc.foo.example.com") && (Path("/abc-foo-example-com") || PathPrefix("/abc-foo-example-com/"))
ROUTER   key=httproute-default-attaches-to-empty-hostname-gw-default-my-gateway-ep-web-0-96396bbe1d7c16e4e2d5 service=httproute-default-attaches-to-empty-hostname-gw-default-my-gateway-ep-web-0-96396bbe1d7c16e4e2d5-wrr priority=11501 rule=!Host("**.example.com") && (Path("/empty-hostname") || PathPrefix("/empty-hostname/"))
DISPATCH GET http://unrelated.test/empty-hostname -> 200 servedBy=BROADER-BACKEND-ROUTER(empty-hostname) rule=!Host("**.example.com") && (Path("/empty-hostname") || PathPrefix("/empty-hostname/"))
DISPATCH GET http://abc.foo.example.com/empty-hostname -> 404 servedBy=<no router> rule=
DISPATCH GET http://abc.foo.example.com/abc-foo-example-com -> 200 servedBy=SPECIFIC-BACKEND-ROUTER(abc-foo-example-com) rule=Host("abc.foo.example.com") && (Path("/abc-foo-example-com") || PathPrefix("/abc-foo-example-com/"))
DISPATCH GET http://unrelated.test/abc-foo-example-com -> 404 servedBy=<no router> rule=
```

In-memory exchange of rule/priority/TLS between the two router objects (router keys and services left in place) — the authored tests stay green, i.e. they do not key a rule to a router name or service:

```console
##### 3. authored tests after exchanging rule/priority/TLS between the two router objects in the loaded configuration (hook injected after loadConfigurationFromGateways)
HOOK-INJECTED into 1 authored test bodies
    kubernetes_test.go:9794: VERIFY hook: exchanged rule/priority/TLS between the backend routers of listeners "empty-hostname" and "wildcard-example-com"
    kubernetes_test.go:9794: VERIFY hook: exchanged rule/priority/TLS between the backend routers of listeners "empty-hostname" and "abc-foo-example-com"
    kubernetes_test.go:9794: VERIFY hook: exchanged rule/priority/TLS between the backend routers of listeners "empty-hostname" and "abc-foo-example-com"
--- PASS: TestLoadRoutes_listenerIsolation (0.00s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoute_attached_to_every_listener_is_only_served_by_the_most_specific_one (0.11s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_with_hostnames_intersecting_the_listener_hostnames (0.11s)
    --- PASS: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_listeners_with_and_without_hostname (0.11s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_listeners_with_hostnames_of_increasing_specificity (0.11s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners_sharing_the_port_detect_the_misdirected_requests (0.11s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.132s
```

Dispatch on that exchanged configuration does tell the two apart (the broader path is now served by the router of the more specific route), and still rejects the more specific hostname on the broader path:

```console
CONFIG   fixture=httproute/with_listener_isolation.yml rule-exchange-in-loaded-config=true
DISPATCH GET http://unrelated.test/empty-hostname -> 200 servedBy=SPECIFIC-BACKEND-ROUTER(abc-foo-example-com) rule=!Host("**.example.com") && (Path("/empty-hostname") || PathPrefix("/empty-hostname/"))
DISPATCH GET http://abc.foo.example.com/empty-hostname -> 404 servedBy=<no router> rule=
DISPATCH GET http://abc.foo.example.com/abc-foo-example-com -> 200 servedBy=BROADER-BACKEND-ROUTER(empty-hostname) rule=Host("abc.foo.example.com") && (Path("/abc-foo-example-com") || PathPrefix("/abc-foo-example-com/"))
DISPATCH GET http://unrelated.test/abc-foo-example-com -> 404 servedBy=<no router> rule=
```

Fixture exchange of the parent bindings between the broader route and the most specific route, authored test code untouched — the exact rule collection fails, naming the exclusion that moved to the other route:

```console
##### 4. authored tests, untouched test code, after a fixture exchange
FIXTURE-EXCHANGE pkg/provider/kubernetes/gateway/fixtures/httproute/with_listener_isolation.yml: parents between route attaches-to-empty-hostname (listener empty-hostname) and route attaches-to-abc-foo-example-com (listener abc-foo-example-com)
        	Error Trace:	/home/ubuntu/wt/b52f8b88/pkg/provider/kubernetes/gateway/kubernetes_test.go:9799
        	Error:      	elements differ
        	            	extra elements in list A:
        	            	extra elements in list B:
--- FAIL: TestLoadRoutes_listenerIsolation (0.00s)
    --- PASS: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_listeners_with_and_without_hostname (0.24s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_with_hostnames_intersecting_the_listener_hostnames (0.21s)
    --- FAIL: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_listeners_with_hostnames_of_increasing_specificity (0.24s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners_sharing_the_port_detect_the_misdirected_requests (0.24s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoute_attached_to_every_listener_is_only_served_by_the_most_specific_one (0.25s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.294s
```

```console
        	            	extra elements in list A:
        	            	 (gateway.routerSummary) {
        	            	  Rule: (string) (len=86) "!Host(\"**.example.com\") && (Path(\"/empty-hostname\") || PathPrefix(\"/empty-hostname/\"))",
        	            	 (gateway.routerSummary) {
        	            	  Rule: (string) (len=100) "Host(\"abc.foo.example.com\") && (Path(\"/abc-foo-example-com\") || PathPrefix(\"/abc-foo-example-com/\"))",
        	            	extra elements in list B:
        	            	 (gateway.routerSummary) {
        	            	  Rule: (string) (len=96) "!Host(\"**.example.com\") && (Path(\"/abc-foo-example-com\") || PathPrefix(\"/abc-foo-example-com/\"))",
        	            	 (gateway.routerSummary) {
        	            	  Rule: (string) (len=90) "Host(\"abc.foo.example.com\") && (Path(\"/empty-hostname\") || PathPrefix(\"/empty-hostname/\"))",
```

Remaining exchanges (`spec.rules` blocks on the HTTP fixture; parent bindings between the broader route and the `*.example.com` route; gRPC fixture; HTTPS fixture):

```console
##### 5. authored tests, untouched test code, after a fixture exchange
FIXTURE-EXCHANGE pkg/provider/kubernetes/gateway/fixtures/httproute/with_listener_isolation.yml: rules between route attaches-to-empty-hostname (listener empty-hostname) and route attaches-to-abc-foo-example-com (listener abc-foo-example-com)
        	Error Trace:	/home/ubuntu/wt/b52f8b88/pkg/provider/kubernetes/gateway/kubernetes_test.go:9799
--- FAIL: TestLoadRoutes_listenerIsolation (0.00s)
    --- FAIL: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_listeners_with_hostnames_of_increasing_specificity (0.24s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoute_attached_to_every_listener_is_only_served_by_the_most_specific_one (0.24s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_with_hostnames_intersecting_the_listener_hostnames (0.23s)
    --- PASS: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_listeners_with_and_without_hostname (0.24s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners_sharing_the_port_detect_the_misdirected_requests (0.24s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.294s
##### 6. authored tests, untouched test code, after a fixture exchange
FIXTURE-EXCHANGE pkg/provider/kubernetes/gateway/fixtures/httproute/with_listener_isolation.yml: parents between route attaches-to-empty-hostname (listener empty-hostname) and route attaches-to-wildcard-example-com (listener wildcard-example-com)
        	Error Trace:	/home/ubuntu/wt/b52f8b88/pkg/provider/kubernetes/gateway/kubernetes_test.go:9799
--- FAIL: TestLoadRoutes_listenerIsolation (0.00s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoute_attached_to_every_listener_is_only_served_by_the_most_specific_one (0.22s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners_sharing_the_port_detect_the_misdirected_requests (0.22s)
    --- FAIL: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_listeners_with_hostnames_of_increasing_specificity (0.23s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_with_hostnames_intersecting_the_listener_hostnames (0.22s)
    --- PASS: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_listeners_with_and_without_hostname (0.23s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.300s
##### 7. authored tests, untouched test code, after a fixture exchange
FIXTURE-EXCHANGE pkg/provider/kubernetes/gateway/fixtures/grpcroute/with_listener_isolation.yml: parents between route attaches-to-empty-hostname (listener empty-hostname) and route attaches-to-wildcard-example-com (listener wildcard-example-com)
--- PASS: TestLoadRoutes_listenerIsolation (0.00s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoute_attached_to_every_listener_is_only_served_by_the_most_specific_one (0.23s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_listeners_with_hostnames_of_increasing_specificity (0.24s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_with_hostnames_intersecting_the_listener_hostnames (0.24s)
    --- PASS: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_listeners_with_and_without_hostname (0.24s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners_sharing_the_port_detect_the_misdirected_requests (0.24s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.307s
##### 8. authored tests, untouched test code, after a fixture exchange
FIXTURE-EXCHANGE pkg/provider/kubernetes/gateway/fixtures/httproute/with_https_listeners_misdirected_requests.yml: parents between route https-app-3 (listener https-with-wildcard-hostname) and route https-app-4 (listener https-with-hostname-matching-wildcard)
--- PASS: TestLoadRoutes_listenerIsolation (0.00s)
    --- PASS: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_listeners_with_and_without_hostname (0.12s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoute_attached_to_every_listener_is_only_served_by_the_most_specific_one (0.12s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_with_hostnames_intersecting_the_listener_hostnames (0.11s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners_sharing_the_port_detect_the_misdirected_requests (0.12s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_listeners_with_hostnames_of_increasing_specificity (0.12s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.154s
##### worktree after restore: 0 modified files
```

### C1 on evalon/traefik-ga-12fa8560

Authored tests, untouched:

```console
##### 1. authored tests, untouched
--- PASS: TestLoadHTTPRoutes_listenerIsolation (0.24s)
--- PASS: TestLoadHTTPRoutes_httpsListenerIsolation (0.24s)
--- PASS: TestLoadGRPCRoutes_listenerIsolation (0.24s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.305s
```

Dispatch on the configuration the authored HTTP test loads (not exchanged). The router generated for the HTTPRoute attached to the broader listener is the one carrying the exclusion rule of the asserted collection, and a request for the more specific hostname with that route's own path is rejected:

```console
CONFIG   fixture=httproute/with_listener_isolation.yml rule-exchange-in-loaded-config=false
ROUTER   key=httproute-default-attaches-to-abc-foo-example-com-gw-default-my-gateway-ep-web-0-ee0d5da78ccdb047521a service=httproute-default-attaches-to-abc-foo-example-com-gw-default-my-gateway-ep-web-0-ee0d5da78ccdb047521a-wrr priority=12020 rule=Host("abc.foo.example.com") && (Path("/abc-foo-example-com") || PathPrefix("/abc-foo-example-com/"))
ROUTER   key=httproute-default-attaches-to-empty-hostname-gw-default-my-gateway-ep-web-0-89c9a004c313ebd95998 service=httproute-default-attaches-to-empty-hostname-gw-default-my-gateway-ep-web-0-89c9a004c313ebd95998-wrr priority=11501 rule=!Host("**.example.com") && !Host("**.foo.example.com") && !Host("abc.foo.example.com") && (Path("/empty-hostname") || PathPrefix("/empty-hostname/"))
DISPATCH GET http://unrelated.test/empty-hostname -> 200 servedBy=BROADER-BACKEND-ROUTER(empty-hostname) rule=!Host("**.example.com") && !Host("**.foo.example.com") && !Host("abc.foo.example.com") && (Path("/empty-hostname") || PathPrefix("/empty-hostname/"))
DISPATCH GET http://abc.foo.example.com/empty-hostname -> 404 servedBy=<no router> rule=
DISPATCH GET http://abc.foo.example.com/abc-foo-example-com -> 200 servedBy=SPECIFIC-BACKEND-ROUTER(abc-foo-example-com) rule=Host("abc.foo.example.com") && (Path("/abc-foo-example-com") || PathPrefix("/abc-foo-example-com/"))
DISPATCH GET http://unrelated.test/abc-foo-example-com -> 404 servedBy=<no router> rule=
```

In-memory exchange of rule/priority/TLS between the two router objects (router keys and services left in place) — the authored tests stay green, i.e. they do not key a rule to a router name or service:

```console
##### 3. authored tests after exchanging rule/priority/TLS between the two router objects in the loaded configuration (hook injected after loadConfigurationFromGateways)
HOOK-INJECTED into 3 authored test bodies
--- PASS: TestLoadHTTPRoutes_httpsListenerIsolation (0.18s)
    kubernetes_test.go:10801: VERIFY hook: exchanged rule/priority/TLS between the backend routers of listeners "empty-hostname" and "wildcard-example-com"
--- PASS: TestLoadGRPCRoutes_listenerIsolation (0.18s)
    kubernetes_test.go:10634: VERIFY hook: exchanged rule/priority/TLS between the backend routers of listeners "empty-hostname" and "abc-foo-example-com"
--- PASS: TestLoadHTTPRoutes_listenerIsolation (0.19s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.261s
```

Dispatch on that exchanged configuration does tell the two apart (the broader path is now served by the router of the more specific route), and still rejects the more specific hostname on the broader path:

```console
CONFIG   fixture=httproute/with_listener_isolation.yml rule-exchange-in-loaded-config=true
DISPATCH GET http://unrelated.test/empty-hostname -> 200 servedBy=SPECIFIC-BACKEND-ROUTER(abc-foo-example-com) rule=!Host("**.example.com") && !Host("**.foo.example.com") && !Host("abc.foo.example.com") && (Path("/empty-hostname") || PathPrefix("/empty-hostname/"))
DISPATCH GET http://abc.foo.example.com/empty-hostname -> 404 servedBy=<no router> rule=
DISPATCH GET http://abc.foo.example.com/abc-foo-example-com -> 200 servedBy=BROADER-BACKEND-ROUTER(empty-hostname) rule=Host("abc.foo.example.com") && (Path("/abc-foo-example-com") || PathPrefix("/abc-foo-example-com/"))
DISPATCH GET http://unrelated.test/abc-foo-example-com -> 404 servedBy=<no router> rule=
```

Fixture exchange of the parent bindings between the broader route and the most specific route, authored test code untouched — the exact rule collection fails, naming the exclusion that moved to the other route:

```console
##### 4. authored tests, untouched test code, after a fixture exchange
FIXTURE-EXCHANGE pkg/provider/kubernetes/gateway/fixtures/httproute/with_listener_isolation.yml: parents between route attaches-to-empty-hostname (listener empty-hostname) and route attaches-to-abc-foo-example-com (listener abc-foo-example-com)
--- PASS: TestLoadGRPCRoutes_listenerIsolation (0.19s)
        	Error Trace:	/home/ubuntu/wt/12fa8560/pkg/provider/kubernetes/gateway/kubernetes_test.go:10647
        	Error:      	elements differ
        	            	extra elements in list A:
        	            	extra elements in list B:
--- FAIL: TestLoadHTTPRoutes_listenerIsolation (0.19s)
--- PASS: TestLoadHTTPRoutes_httpsListenerIsolation (0.20s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.233s
```

```console
        	            	extra elements in list A:
        	            	 (string) (len=149) "!Host(\"**.example.com\") && !Host(\"**.foo.example.com\") && !Host(\"abc.foo.example.com\") && (Path(\"/empty-hostname\") || PathPrefix(\"/empty-hostname/\"))",
        	            	 (string) (len=100) "Host(\"abc.foo.example.com\") && (Path(\"/abc-foo-example-com\") || PathPrefix(\"/abc-foo-example-com/\"))"
        	            	extra elements in list B:
        	            	 (string) (len=159) "!Host(\"**.example.com\") && !Host(\"**.foo.example.com\") && !Host(\"abc.foo.example.com\") && (Path(\"/abc-foo-example-com\") || PathPrefix(\"/abc-foo-example-com/\"))",
        	            	 (string) (len=90) "Host(\"abc.foo.example.com\") && (Path(\"/empty-hostname\") || PathPrefix(\"/empty-hostname/\"))"
```

Remaining exchanges (`spec.rules` blocks on the HTTP fixture; parent bindings between the broader route and the `*.example.com` route; gRPC fixture; HTTPS fixture):

```console
##### 5. authored tests, untouched test code, after a fixture exchange
FIXTURE-EXCHANGE pkg/provider/kubernetes/gateway/fixtures/httproute/with_listener_isolation.yml: rules between route attaches-to-empty-hostname (listener empty-hostname) and route attaches-to-abc-foo-example-com (listener abc-foo-example-com)
--- PASS: TestLoadGRPCRoutes_listenerIsolation (0.17s)
        	Error Trace:	/home/ubuntu/wt/12fa8560/pkg/provider/kubernetes/gateway/kubernetes_test.go:10647
--- FAIL: TestLoadHTTPRoutes_listenerIsolation (0.17s)
--- PASS: TestLoadHTTPRoutes_httpsListenerIsolation (0.17s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.202s
##### 6. authored tests, untouched test code, after a fixture exchange
FIXTURE-EXCHANGE pkg/provider/kubernetes/gateway/fixtures/httproute/with_listener_isolation.yml: parents between route attaches-to-empty-hostname (listener empty-hostname) and route attaches-to-wildcard-example-com (listener wildcard-example-com)
--- PASS: TestLoadGRPCRoutes_listenerIsolation (0.16s)
        	Error Trace:	/home/ubuntu/wt/12fa8560/pkg/provider/kubernetes/gateway/kubernetes_test.go:10647
--- FAIL: TestLoadHTTPRoutes_listenerIsolation (0.16s)
--- PASS: TestLoadHTTPRoutes_httpsListenerIsolation (0.17s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.236s
##### 7. authored tests, untouched test code, after a fixture exchange
FIXTURE-EXCHANGE pkg/provider/kubernetes/gateway/fixtures/grpcroute/with_listener_isolation.yml: parents between route attaches-to-empty-hostname (listener empty-hostname) and route attaches-to-wildcard-example-com (listener wildcard-example-com)
--- PASS: TestLoadGRPCRoutes_listenerIsolation (0.19s)
--- PASS: TestLoadHTTPRoutes_httpsListenerIsolation (0.19s)
--- PASS: TestLoadHTTPRoutes_listenerIsolation (0.19s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.252s
##### 8. authored tests, untouched test code, after a fixture exchange
FIXTURE-EXCHANGE pkg/provider/kubernetes/gateway/fixtures/httproute/with_https_listener_isolation.yml: parents between route https-test-3 (listener https-with-wildcard-hostname) and route https-test-4 (listener https-with-hostname-matching-wildcard)
--- PASS: TestLoadHTTPRoutes_listenerIsolation (0.15s)
--- PASS: TestLoadGRPCRoutes_listenerIsolation (0.15s)
--- PASS: TestLoadHTTPRoutes_httpsListenerIsolation (0.15s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.196s
##### worktree after restore: 0 modified files
```

### C1 reading

- Refute condition met on all three branches: an executed authored test asserts the exclusion on an identified broader backend route. The identification comes from assertions that fail after exchanging parent bindings or complete `spec.rules` between the broader and the more specific route (steps 4, 5, 6), and from the exact rule collection paired with dispatch on the same configuration, which shows the rule carrying the `!Host(...)` exclusion is the rule of the router generated for the HTTPRoute attached to the broader listener.
- What does hold from the claim, and is not enough to confirm it: no authored test sends a request; the collections ignore router keys and services, so swapping rules between router objects in memory is invisible to them (step 3); and exchanging parent bindings in the gRPC and HTTPS fixtures leaves the authored tests green (steps 7, 8) — I did not establish why beyond that observation. The HTTP case of each branch still provides the route-identified exclusion the claim says is absent.
- The dispatch probe is mine, not the branch's; the 404 for `abc.foo.example.com/empty-hostname` is observed through `httpmuxer.Muxer` with the generated rules, not through a full router factory.

## C2

Claim: with two sibling Gateways providing competing catch-all HTTPS fallbacks, a request using an exact listener hostname as SNI, a Host matching only catch-all listeners, and an unmatched path returns HTTP 404 instead of HTTP 421 for at least one Gateway. Branch `evalon/traefik-ga-fd783057`, HEAD `c27a5d30ec8341d8fe0ce05161b3ad341b19c755`.

```sh
cd ~/repos/traefik
git remote add claims https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation   # same proxy host as origin in the audit VM
git fetch claims 'refs/heads/evalon/*:refs/remotes/claims/evalon/*'
git worktree add --detach ~/wt/fd783057 claims/evalon/traefik-ga-fd783057
unzip eval_tests.zip -d ~/eval        # byte-exact eval fixtures: ~/eval/tests/{eval_https_test.go,eval_isolation_test.go,export_eval_test.go}
go version                            # go1.26.3 linux/amd64
```

```sh
bash verify/repro/c2_run.sh ~/wt/fd783057 ~/eval 10 > verify/logs/c2_run.txt 2>&1
```

`c2_run.sh` copies the eval fixtures and `verify/repro/c2_sibling_gateway_fallback_test.go` into `pkg/provider/kubernetes/gateway/` and runs `go test -tags verify -count=1 -v -run TestVerifyC2 ./pkg/provider/kubernetes/gateway`, then the same with `-count=10`. The probe builds two Gateways `gw-a` and `gw-b` on entry point `websecure`, each with a catch-all HTTPS listener and an exact one (`a.example.com`, `b.example.com`), loads them with the provider, prints the priority-1 routers, and sends requests through the eval harness `evalServeHTTPSFull` (`server.NewConfigurationWatcher` -> TLS manager -> `server.NewRouterFactory` -> `router.GetHTTPSHandler().ServeHTTP`, TLS options picked from the SNI). Each harness mode (strict SNI off, on) is measured twice: unlabelled, and with a response-header middleware on each fallback router naming it.

### C2 fallback selection

Generated fallback routers and TLS options (both runs print the same):

```console
FALLBACK https-listener-default-gw-a-gw-default-gw-a-ep-websecure-0-acb64b785f6569eac2fd rule=PathPrefix("/") priority=1 entryPoints=[websecure] tls.options="" tls.gatewayListeners="*,a.example.com" service=https-listener-default-gw-a-gw-default-gw-a-ep-websecure-0-acb64b785f6569eac2fd
FALLBACK https-listener-default-gw-b-gw-default-gw-b-ep-websecure-0-8bec38ada93978bfcd2a rule=PathPrefix("/") priority=1 entryPoints=[websecure] tls.options="" tls.gatewayListeners="*,b.example.com" service=https-listener-default-gw-b-gw-default-gw-b-ep-websecure-0-8bec38ada93978bfcd2a
TLS options generated by provider: 0
```

Both fallbacks have rule `PathPrefix("/")` and priority 1; they differ only in `tls.gatewayListeners`, which lists the fallback's own Gateway's listeners (`*,a.example.com` versus `*,b.example.com`). No named TLS option is generated, so the TLS-option comparison in `SNICheck` cannot separate the two Gateways. Source of the generated router, `pkg/provider/kubernetes/gateway/listener.go` on the branch:

```go
			rule = `PathPrefix("/")`
			Priority:    1,
			TLS:         &dynamic.RouterTLSConfig{GatewayListeners: listeners[i].TLSHostnames},
```

and the check the selected fallback runs, `pkg/middlewares/snicheck/snicheck.go` on the branch:

```go
	if len(s.gatewayListeners) > 0 {
		hostListener := s.matchingListener(requestdecorator.GetCanonicalHost(req.Context()))
		if hostListener == -1 {
			http.NotFound(rw, req)
			return
		}

		if hostListener != s.matchingListener(req.TLS.ServerName) {
			http.Error(rw, http.StatusText(http.StatusMisdirectedRequest), http.StatusMisdirectedRequest)
			return
		}
	}
```

For `gw-a`'s fallback (`*,a.example.com`), Host `xyz.example.org` and SNI `b.example.com` both resolve to listener `*`, so the check passes and the fallback's 404 service answers. The observed rows below show exactly that: the 404 for B's exact SNI carries `gw-a`'s label.

### C2 production reachability

First run, harness strict-SNI off; then the second mode of the same run, where the other fallback won:

```console
SINGLE   A exact SNI, catch-all Host, unmatched path          sni=a.example.com    host=xyz.example.org  path=/nope -> 421
SIBLING  A exact SNI, catch-all Host, unmatched path          sni=a.example.com    host=xyz.example.org  path=/nope -> unlabelled=421 labelled=421 servedByFallback=""
SIBLING  B exact SNI, catch-all Host, unmatched path          sni=b.example.com    host=xyz.example.org  path=/nope -> unlabelled=404 labelled=404 servedByFallback="gw-default-gw-a-ep-websecure-0-acb64b785f6569eac2fd"
SIBLING  control: catch-all SNI=Host, unmatched path          sni=xyz.example.org  host=xyz.example.org  path=/nope -> unlabelled=404 labelled=404 servedByFallback="gw-default-gw-a-ep-websecure-0-acb64b785f6569eac2fd"
SIBLING  control: A exact SNI=Host, unmatched path            sni=a.example.com    host=a.example.com    path=/nope -> unlabelled=404 labelled=404 servedByFallback="gw-default-gw-a-ep-websecure-0-acb64b785f6569eac2fd"
SIBLING  control: catch-all SNI, A exact Host, unmatched path sni=xyz.example.org  host=a.example.com    path=/nope -> unlabelled=421 labelled=421 servedByFallback=""
SIBLING  control: catch-all SNI, B exact Host, unmatched path sni=xyz.example.org  host=b.example.com    path=/nope -> unlabelled=404 labelled=404 servedByFallback="gw-default-gw-a-ep-websecure-0-acb64b785f6569eac2fd"
RESULT   A-direction=421 B-direction=404 (claim expects 421 for both; 404 on either = crossing reached a fallback)
SINGLE   A exact SNI, catch-all Host, unmatched path          sni=a.example.com    host=xyz.example.org  path=/nope -> 421
SIBLING  A exact SNI, catch-all Host, unmatched path          sni=a.example.com    host=xyz.example.org  path=/nope -> unlabelled=404 labelled=404 servedByFallback="gw-default-gw-b-ep-websecure-0-8bec38ada93978bfcd2a"
SIBLING  B exact SNI, catch-all Host, unmatched path          sni=b.example.com    host=xyz.example.org  path=/nope -> unlabelled=421 labelled=421 servedByFallback=""
SIBLING  control: catch-all SNI=Host, unmatched path          sni=xyz.example.org  host=xyz.example.org  path=/nope -> unlabelled=404 labelled=404 servedByFallback="gw-default-gw-b-ep-websecure-0-8bec38ada93978bfcd2a"
SIBLING  control: A exact SNI=Host, unmatched path            sni=a.example.com    host=a.example.com    path=/nope -> unlabelled=404 labelled=404 servedByFallback="gw-default-gw-b-ep-websecure-0-8bec38ada93978bfcd2a"
SIBLING  control: catch-all SNI, A exact Host, unmatched path sni=xyz.example.org  host=a.example.com    path=/nope -> unlabelled=404 labelled=404 servedByFallback="gw-default-gw-b-ep-websecure-0-8bec38ada93978bfcd2a"
SIBLING  control: catch-all SNI, B exact Host, unmatched path sni=xyz.example.org  host=b.example.com    path=/nope -> unlabelled=421 labelled=421 servedByFallback=""
RESULT   A-direction=404 B-direction=421 (claim expects 421 for both; 404 on either = crossing reached a fallback)
--- PASS: TestVerifyC2_SiblingGatewayFallback (0.73s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.889s
```

- Baseline with a single Gateway: the same crossing returns 421.
- With the sibling Gateway present, one of the two crossing directions returns 404, the 404 is served by the other Gateway's fallback (`servedByFallback` names it), and nothing rejects the request before that fallback. The unlabelled status is the same as the labelled one, so the label middleware does not change the result.
- The reverse crossing (catch-all SNI, the other Gateway's exact hostname as Host) shows the same 404 on the same side.

Which Gateway is affected depends on which equal-priority fallback the muxer tries first, and that changes between configuration builds:

```console
##### 2. tally of the two crossing directions over 10 runs
      1 RESULT   A-direction=404 B-direction=421 (claim expects 421 for both; 404 on either = crossing reached a fallback)
     19 RESULT   A-direction=421 B-direction=404 (claim expects 421 for both; 404 on either = crossing reached a fallback)
##### worktree after restore: 0 modified files

```

(10 test runs x 2 harness modes = 20 builds.) In every one of the 20, exactly one direction returned 404.

### C2 reading

Both parts hold. Impact as observed: on a port shared by two Gateways that each have a catch-all HTTPS listener and an exact-hostname listener, a client whose TLS connection was opened for one Gateway's exact hostname and whose request carries a Host served only by the catch-all listeners gets 404 instead of 421 on an unmatched path for one of the two Gateways, and which one is not stable. A single Gateway with the same listeners answers 421. Not measured: matched paths, real sockets (the harness sets `req.TLS.ServerName` and the SNI-selected TLS option in-process).

## C3

Claim: the authored HTTPS tests assert response statuses synthesized from raw TLS options by test-local SNI rejection logic that bypasses production TLS-option aggregation and request handling. Branch `evalon/traefik-ga-a6535cd8`, HEAD `cae6235c3aebf508ac49b2d6509b7fbc6314b445`.

```sh
cd ~/repos/traefik
git remote add claims https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation   # same proxy host as origin in the audit VM
git fetch claims 'refs/heads/evalon/*:refs/remotes/claims/evalon/*'
git worktree add --detach ~/wt/a6535cd8 claims/evalon/traefik-ga-a6535cd8
unzip eval_tests.zip -d ~/eval        # byte-exact eval fixtures: ~/eval/tests/{eval_https_test.go,eval_isolation_test.go,export_eval_test.go}
go version                            # go1.26.3 linux/amd64
```

```sh
bash verify/repro/c3_run.sh ~/wt/a6535cd8 ~/eval > verify/logs/c3_run.txt 2>&1
```

`c3_run.sh` copies the eval fixtures and the two `verify/repro/c3_*_test.go` files into `pkg/provider/kubernetes/gateway/`, runs the authored test (`go test -count=1 -v -run '^TestListenerIsolation_HTTPSDetectMisdirectedRequests$'`), runs the comparison probe (`go test -tags verify -count=1 -v -run TestVerifyC3`), then repeats both after a one-line mutation of production `pkg/middlewares/snicheck/snicheck.go`, and restores the file.

### C3 source of the asserted status

`pkg/provider/kubernetes/gateway/listener_isolation_test.go` on the branch, loop body of `TestListenerIsolation_HTTPSDetectMisdirectedRequests`:

```go
		routerName := serveListenerIsolationRequest(mux, "https://"+test.host+"/detect-misdirected-requests", nil)
		require.NotEmpty(t, routerName, "%s -> %s", test.serverName, test.host)

		router := conf.HTTP.Routers[routerName]

		// Mimics the router SNI check, which compares the TLS options selected by the SNI with the router ones.
		statusCode := http.StatusOK
		switch {
		case sniOptions(test.serverName) != router.TLS.Options:
			statusCode = http.StatusMisdirectedRequest
		case conf.HTTP.Services[router.Service].Weighted != nil && conf.HTTP.Services[router.Service].Weighted.Services[0].Status != nil:
			statusCode = *conf.HTTP.Services[router.Service].Weighted.Services[0].Status
		}

		assert.Equal(t, test.statusCode, statusCode, "%s -> %s", test.serverName, test.host)
```

`serveListenerIsolationRequest` only returns the name of the matched router from a test-built muxer. `sniOptions` comes from the test helper `buildSNITLSOptions`, which maps `httpmuxer.ParseDomains(router.Rule)` to the raw `router.TLS.Options` of the provider output. No `SNICheck`, router factory or `pkg/server` aggregation runs. All 15 rows of the table carry a `statusCode`, so every asserted status (200, 404, 421) is produced by this `switch`:

```console
##### 1. where the authored test gets its asserted status
139:		statusCode int
142:		{serverName: "example.org", host: "example.org", statusCode: http.StatusOK, route: "detect-misdirected-requests-1"},
143:		{serverName: "example.org", host: "second-example.org", statusCode: http.StatusMisdirectedRequest},
144:		{serverName: "example.org", host: "unknown-example.org", statusCode: http.StatusNotFound},
146:		{serverName: "second-example.org", host: "second-example.org", statusCode: http.StatusOK, route: "detect-misdirected-requests-2"},
147:		{serverName: "second-example.org", host: "example.org", statusCode: http.StatusMisdirectedRequest},
148:		{serverName: "second-example.org", host: "unknown-example.org", statusCode: http.StatusMisdirectedRequest},
150:		{serverName: "third-example.wildcard.org", host: "third-example.wildcard.org", statusCode: http.StatusOK, route: "detect-misdirected-requests-3"},
151:		{serverName: "third-example.wildcard.org", host: "fith-example.wildcard.org", statusCode: http.StatusOK, route: "detect-misdirected-requests-3"},
152:		{serverName: "third-example.wildcard.org", host: "fourth-example.wildcard.org", statusCode: http.StatusMisdirectedRequest},
153:		{serverName: "third-example.wildcard.org", host: "second-example.org", statusCode: http.StatusMisdirectedRequest},
154:		{serverName: "third-example.wildcard.org", host: "unknown-example.org", statusCode: http.StatusMisdirectedRequest},
156:		{serverName: "fourth-example.wildcard.org", host: "fourth-example.wildcard.org", statusCode: http.StatusOK, route: "detect-misdirected-requests-4"},
157:		{serverName: "fourth-example.wildcard.org", host: "fith-example.wildcard.org", statusCode: http.StatusMisdirectedRequest},
159:		{serverName: "unknown-example.org", host: "example.org", statusCode: http.StatusOK, route: "detect-misdirected-requests-1"},
160:		{serverName: "unknown-example.org", host: "unknown-example.org", statusCode: http.StatusNotFound},
170:		statusCode := http.StatusOK
173:			statusCode = http.StatusMisdirectedRequest
175:			statusCode = *conf.HTTP.Services[router.Service].Weighted.Services[0].Status
178:		assert.Equal(t, test.statusCode, statusCode, "%s -> %s", test.serverName, test.host)
```

Authored test, untouched:

```console
##### 2. authored test, untouched
=== RUN   TestListenerIsolation_HTTPSDetectMisdirectedRequests
--- PASS: TestListenerIsolation_HTTPSDetectMisdirectedRequests (0.11s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.251s
```

### C3 test-local status against production handling

`verify/repro/c3_export_verify_test.go` replays the loop body above verbatim with the branch's own helpers; `verify/repro/c3_testlocal_status_vs_production_test.go` prints it next to the status of the eval harness `evalServeHTTPSFull` (`server.NewConfigurationWatcher` -> TLS manager -> `server.NewRouterFactory` -> `router.GetHTTPSHandler().ServeHTTP`) for the same fixture, `httproute/with_https_listener_detect_misdirected_requests.yml`. `asserted` is the authored table value (0 = extra row of mine):

```console
##### 3. test-local status vs production handling, untouched production code
{"level":"error","time":"2026-11-09T02:05:34Z","message":"On EntryPoint \"websecure\", router \"httproute-default-detect-misdirected-requests-3-gw-default-my-gateway-ep-websecure-0-fbdb5a70df937e6042db@kubernetesgateway\", with no domains in its rule, is configured with a TLS options different than the default one, default TLSOp
sni="example.org"                host="example.org"                asserted=200 test-local=200 production=200 router=httproute-default-detect-misdirected-requests-1-gw-default-my-gateway-ep-websecure-0-5e422da006de204fb3d1
sni="example.org"                host="second-example.org"         asserted=421 test-local=421 production=421 router=httproute-default-detect-misdirected-requests-2-gw-default-my-gateway-ep-websecure-0-21fb336a7f1a7ded7c83
sni="example.org"                host="unknown-example.org"        asserted=404 test-local=404 production=404 router=listener-isolation-gw-default-my-gateway-ep-websecure-c3edf98a3c7e2218ca39
sni="second-example.org"         host="second-example.org"         asserted=200 test-local=200 production=200 router=httproute-default-detect-misdirected-requests-2-gw-default-my-gateway-ep-websecure-0-21fb336a7f1a7ded7c83
sni="second-example.org"         host="example.org"                asserted=421 test-local=421 production=421 router=httproute-default-detect-misdirected-requests-1-gw-default-my-gateway-ep-websecure-0-5e422da006de204fb3d1
sni="second-example.org"         host="unknown-example.org"        asserted=421 test-local=421 production=421 router=listener-isolation-gw-default-my-gateway-ep-websecure-c3edf98a3c7e2218ca39
sni="third-example.wildcard.org" host="third-example.wildcard.org" asserted=200 test-local=200 production=421 router=httproute-default-detect-misdirected-requests-3-gw-default-my-gateway-ep-websecure-0-fbdb5a70df937e6042db  <-- DIVERGES
sni="third-example.wildcard.org" host="fith-example.wildcard.org"  asserted=200 test-local=200 production=421 router=httproute-default-detect-misdirected-requests-3-gw-default-my-gateway-ep-websecure-0-fbdb5a70df937e6042db  <-- DIVERGES
sni="third-example.wildcard.org" host="fourth-example.wildcard.org" asserted=421 test-local=421 production=421 router=httproute-default-detect-misdirected-requests-4-gw-default-my-gateway-ep-websecure-0-043461846a86a57eac66
sni="third-example.wildcard.org" host="second-example.org"         asserted=421 test-local=421 production=421 router=httproute-default-detect-misdirected-requests-2-gw-default-my-gateway-ep-websecure-0-21fb336a7f1a7ded7c83
sni="third-example.wildcard.org" host="unknown-example.org"        asserted=421 test-local=421 production=421 router=listener-isolation-gw-default-my-gateway-ep-websecure-c3edf98a3c7e2218ca39
sni="fourth-example.wildcard.org" host="fourth-example.wildcard.org" asserted=200 test-local=200 production=200 router=httproute-default-detect-misdirected-requests-4-gw-default-my-gateway-ep-websecure-0-043461846a86a57eac66
sni="fourth-example.wildcard.org" host="fith-example.wildcard.org"  asserted=421 test-local=421 production=421 router=httproute-default-detect-misdirected-requests-3-gw-default-my-gateway-ep-websecure-0-fbdb5a70df937e6042db
sni="unknown-example.org"        host="example.org"                asserted=200 test-local=200 production=200 router=httproute-default-detect-misdirected-requests-1-gw-default-my-gateway-ep-websecure-0-5e422da006de204fb3d1
sni="unknown-example.org"        host="unknown-example.org"        asserted=404 test-local=404 production=404 router=listener-isolation-gw-default-my-gateway-ep-websecure-c3edf98a3c7e2218ca39
sni="a.b.wildcard.org"           host="a.b.wildcard.org"           asserted=  0 test-local=200 production=421 router=httproute-default-detect-misdirected-requests-3-gw-default-my-gateway-ep-websecure-0-fbdb5a70df937e6042db  <-- DIVERGES
sni="a.b.wildcard.org"           host="second-example.org"         asserted=  0 test-local=421 production=421 router=httproute-default-detect-misdirected-requests-2-gw-default-my-gateway-ep-websecure-0-21fb336a7f1a7ded7c83
sni="second-example.org"         host="a.b.wildcard.org"           asserted=  0 test-local=421 production=421 router=httproute-default-detect-misdirected-requests-3-gw-default-my-gateway-ep-websecure-0-fbdb5a70df937e6042db
sni="SECOND-EXAMPLE.ORG"         host="second-example.org"         asserted=  0 test-local=200 production=200 router=httproute-default-detect-misdirected-requests-2-gw-default-my-gateway-ep-websecure-0-21fb336a7f1a7ded7c83
sni="second-example.org."        host="second-example.org"         asserted=  0 test-local=421 production=421 router=httproute-default-detect-misdirected-requests-2-gw-default-my-gateway-ep-websecure-0-21fb336a7f1a7ded7c83
sni=""                           host="second-example.org"         asserted=  0 test-local=421 production=421 router=httproute-default-detect-misdirected-requests-2-gw-default-my-gateway-ep-websecure-0-21fb336a7f1a7ded7c83
sni=""                           host="example.org"                asserted=  0 test-local=200 production=200 router=httproute-default-detect-misdirected-requests-1-gw-default-my-gateway-ep-websecure-0-5e422da006de204fb3d1
rows=22 diverged=3
--- PASS: TestVerifyC3_TestLocalVsProduction (0.26s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.500s
```

Two asserted rows diverge: for SNI `third-example.wildcard.org` with Host `third-example.wildcard.org` or `fith-example.wildcard.org` the authored test asserts 200, and production handling answers 421. The first line of the block is why: production aggregation (`pkg/server/aggregator.go:288`) finds no domain in the wildcard listener's router rule and resets that router to the default TLS options. A verbose run of the same probe shows the production `SNICheck` rejecting on that basis:

```console
{"level":"debug","routerName":"websecure-conflicted-httproute-default-detect-misdirected-requests-3-gw-default-my-gateway-ep-websecure-0-fbdb5a70df937e6042db@kubernetesgateway","req.Host":"third-example.wildcard.org","req.TLS.ServerName":"third-example.wildcard.org","time":"2026-11-09T02:09:24Z","message":"TLS options difference: SNI:listener-isolation-gw-default-my-gateway-ep-websecure-listener-https-with-wildcard-hostname-80755fc5cf00184ba584@kubernetesgateway, Header:default"}
```

(line 19 of `verify/logs/c3_probe_verbose.txt`, the unfiltered output of `go test -tags verify -count=1 -v -run TestVerifyC3 ./pkg/provider/kubernetes/gateway` in the worktree with the eval fixtures and the two probe files copied in, as `c3_run.sh` does.) The test-local calculation reads the raw option name and never sees that reset.

### C3 mutation: production SNICheck never answers 421

```console
##### 4. mutation: production SNICheck never answers 421
-	if s.tlsOptionsName != tlsOptionsNameUsed {
+	if false && s.tlsOptionsName != tlsOptionsNameUsed {
##### 4a. authored test under the mutation
=== RUN   TestListenerIsolation_HTTPSDetectMisdirectedRequests
--- PASS: TestListenerIsolation_HTTPSDetectMisdirectedRequests (0.17s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.260s
```

The authored test passes with production SNI rejection disabled. Production handling under the same mutation:

```console
##### 4b. test-local status vs production handling under the mutation
{"level":"error","time":"2026-11-09T02:05:56Z","message":"On EntryPoint \"websecure\", router \"httproute-default-detect-misdirected-requests-3-gw-default-my-gateway-ep-websecure-0-fbdb5a70df937e6042db@kubernetesgateway\", with no domains in its rule, is configured with a TLS options different than the default one, default TLSOp
sni="example.org"                host="example.org"                asserted=200 test-local=200 production=200 router=httproute-default-detect-misdirected-requests-1-gw-default-my-gateway-ep-websecure-0-5e422da006de204fb3d1
sni="example.org"                host="second-example.org"         asserted=421 test-local=421 production=200 router=httproute-default-detect-misdirected-requests-2-gw-default-my-gateway-ep-websecure-0-21fb336a7f1a7ded7c83  <-- DIVERGES
sni="example.org"                host="unknown-example.org"        asserted=404 test-local=404 production=404 router=listener-isolation-gw-default-my-gateway-ep-websecure-c3edf98a3c7e2218ca39
sni="second-example.org"         host="second-example.org"         asserted=200 test-local=200 production=200 router=httproute-default-detect-misdirected-requests-2-gw-default-my-gateway-ep-websecure-0-21fb336a7f1a7ded7c83
sni="second-example.org"         host="example.org"                asserted=421 test-local=421 production=200 router=httproute-default-detect-misdirected-requests-1-gw-default-my-gateway-ep-websecure-0-5e422da006de204fb3d1  <-- DIVERGES
sni="second-example.org"         host="unknown-example.org"        asserted=421 test-local=421 production=404 router=listener-isolation-gw-default-my-gateway-ep-websecure-c3edf98a3c7e2218ca39  <-- DIVERGES
sni="third-example.wildcard.org" host="third-example.wildcard.org" asserted=200 test-local=200 production=200 router=httproute-default-detect-misdirected-requests-3-gw-default-my-gateway-ep-websecure-0-fbdb5a70df937e6042db
sni="third-example.wildcard.org" host="fith-example.wildcard.org"  asserted=200 test-local=200 production=200 router=httproute-default-detect-misdirected-requests-3-gw-default-my-gateway-ep-websecure-0-fbdb5a70df937e6042db
sni="third-example.wildcard.org" host="fourth-example.wildcard.org" asserted=421 test-local=421 production=200 router=httproute-default-detect-misdirected-requests-4-gw-default-my-gateway-ep-websecure-0-043461846a86a57eac66  <-- DIVERGES
sni="third-example.wildcard.org" host="second-example.org"         asserted=421 test-local=421 production=200 router=httproute-default-detect-misdirected-requests-2-gw-default-my-gateway-ep-websecure-0-21fb336a7f1a7ded7c83  <-- DIVERGES
sni="third-example.wildcard.org" host="unknown-example.org"        asserted=421 test-local=421 production=404 router=listener-isolation-gw-default-my-gateway-ep-websecure-c3edf98a3c7e2218ca39  <-- DIVERGES
sni="fourth-example.wildcard.org" host="fourth-example.wildcard.org" asserted=200 test-local=200 production=200 router=httproute-default-detect-misdirected-requests-4-gw-default-my-gateway-ep-websecure-0-043461846a86a57eac66
sni="fourth-example.wildcard.org" host="fith-example.wildcard.org"  asserted=421 test-local=421 production=200 router=httproute-default-detect-misdirected-requests-3-gw-default-my-gateway-ep-websecure-0-fbdb5a70df937e6042db  <-- DIVERGES
sni="unknown-example.org"        host="example.org"                asserted=200 test-local=200 production=200 router=httproute-default-detect-misdirected-requests-1-gw-default-my-gateway-ep-websecure-0-5e422da006de204fb3d1
sni="unknown-example.org"        host="unknown-example.org"        asserted=404 test-local=404 production=404 router=listener-isolation-gw-default-my-gateway-ep-websecure-c3edf98a3c7e2218ca39
sni="a.b.wildcard.org"           host="a.b.wildcard.org"           asserted=  0 test-local=200 production=200 router=httproute-default-detect-misdirected-requests-3-gw-default-my-gateway-ep-websecure-0-fbdb5a70df937e6042db
sni="a.b.wildcard.org"           host="second-example.org"         asserted=  0 test-local=421 production=200 router=httproute-default-detect-misdirected-requests-2-gw-default-my-gateway-ep-websecure-0-21fb336a7f1a7ded7c83  <-- DIVERGES
sni="second-example.org"         host="a.b.wildcard.org"           asserted=  0 test-local=421 production=200 router=httproute-default-detect-misdirected-requests-3-gw-default-my-gateway-ep-websecure-0-fbdb5a70df937e6042db  <-- DIVERGES
sni="SECOND-EXAMPLE.ORG"         host="second-example.org"         asserted=  0 test-local=200 production=200 router=httproute-default-detect-misdirected-requests-2-gw-default-my-gateway-ep-websecure-0-21fb336a7f1a7ded7c83
sni="second-example.org."        host="second-example.org"         asserted=  0 test-local=421 production=200 router=httproute-default-detect-misdirected-requests-2-gw-default-my-gateway-ep-websecure-0-21fb336a7f1a7ded7c83  <-- DIVERGES
sni=""                           host="second-example.org"         asserted=  0 test-local=421 production=200 router=httproute-default-detect-misdirected-requests-2-gw-default-my-gateway-ep-websecure-0-21fb336a7f1a7ded7c83  <-- DIVERGES
sni=""                           host="example.org"                asserted=  0 test-local=200 production=200 router=httproute-default-detect-misdirected-requests-1-gw-default-my-gateway-ep-websecure-0-5e422da006de204fb3d1
rows=22 diverged=11
--- PASS: TestVerifyC3_TestLocalVsProduction (0.10s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.146s
##### worktree after restore: 0 modified files

```

Seven asserted 421 rows now answer 200 or 404 in production while the authored test still reports them as 421.

### C3 reading

The claim holds: every status the authored HTTPS test asserts is computed in the test from raw `router.TLS.Options`, bypassing production TLS-option aggregation and request handling. Impact as observed: the test stays green when production rejection is removed, and it already asserts 200 for two requests to the wildcard HTTPS listener (`*.wildcard.org`, sharing port 443 with three other listeners) that production handling answers with 421, with SNI and Host both matching the listener. Not measured: real sockets (the harness sets `req.TLS.ServerName` and the SNI-selected TLS option in-process); whether strict-SNI entry points behave differently for this fixture.
