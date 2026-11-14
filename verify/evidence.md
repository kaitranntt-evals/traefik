Observations for the audit of `traefik-secret-references-in-middlewares-perfect` (84614aa0c). Go 1.26.3 linux/amd64. Credential hashes in pasted output are replaced by `<redacted>`; all credentials are throwaway test values.

## C1

Branch: `evalon/traefik-se-df0ea15d`.

Setup shared by the claims targeting `evalon/traefik-se-df0ea15d` (repeated here so the section replays alone):

```sh
git remote add claims https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git fetch claims evalon/traefik-se-df0ea15d && git worktree add ~/wt/df0ea15d claims/evalon/traefik-se-df0ea15d   # 462a74079
unzip eval_tests.zip -d ~/audit        # fixtures land in ~/audit/tests
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests          # mode "exact": fixtures byte-exact, nothing else
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests bridge   # mode "bridge": + verify/bridge/bridge_*_test.go
```

Why two modes: on this branch the loaders return `(configuration, secrets)` and `Provider.Provide` then runs `dynamic.Secrets.Replace(ProviderName, secrets)` and `conf.SecretsDigest = secrets.Digest()` before hashing and emitting. The archive helper files `export_eval_test.go` / `export_eval_ingress_nginx_test.go` call `loadConfigurationFromCRD` / `loadConfiguration` directly, which on this branch stops before those two publication statements. "bridge" mode points those helper calls at `evalPublishLikeProvide` (in `verify/bridge/`), which runs exactly the three statements of `Provide`; no fixture assertion and no production file is changed. Both modes were run and both outputs are recorded. The pasted lines are filtered from one run per mode of the eleven CRD fixtures named in C1/C4-C8 (and the four ingress-nginx ones); the `ok`/`FAIL` package line and the `DATA RACE reports` count are for that whole run, and all race reports belong to the C8 test.

```sh
cd ~/wt/df0ea15d && go test -race -count=1 -v -run '^TestEval_CRD_SecretRotationAndHashStability$' ./pkg/provider/kubernetes/crd
```

Mode "exact":

```console
--- FAIL: TestEval_CRD_SecretRotationAndHashStability (0.53s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	3.434s
DATA RACE reports: 3
        	Error:      	Should not be: 0x23aed7916fceffcf
        	Messages:   	secret rotation must change the provider change-detection value to trigger reload
        	Error:      	Should be false
        	Messages:   	secret rotation must not be deep-equal, or the configuration watcher drops it
```

Mode "bridge" (helper goes through the publication statements of `Provide`):

```console
--- PASS: TestEval_CRD_SecretRotationAndHashStability (0.55s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	5.132s
DATA RACE reports: 0
```

Live setup (Traefik built from the same worktree, real Kubernetes API from k3s in Docker):

```sh
cd ~/wt/df0ea15d && go build -o ~/audit/traefik-df ./cmd/traefik
docker run -d --name k3s --privileged -p 6443:6443 rancher/k3s server   # kubeconfig copied out of the container
kubectl apply -f docs/content/reference/dynamic-configuration/kubernetes-crd-definition-v1.yml -f verify/live/manifests.yml
~/audit/traefik-df --log.level=DEBUG --api.insecure --entrypoints.web.address=:8880 --entrypoints.traefik.address=:8881 \
  --providers.kubernetescrd.endpoint=https://127.0.0.1:6443 --providers.kubernetescrd.token=... --providers.kubernetescrd.certauthfilepath=... \
  --providers.kubernetesingressnginx.endpoint=https://127.0.0.1:6443 ... > ~/audit/live/traefik.log 2>&1 &
KUBECTL="docker exec -i k3s kubectl" LOG=~/audit/live/traefik.log bash verify/live/probe.sh
```

```console
wrong password: digest -> 401 basic -> 401 file -> 401 map -> 401
== 3. no-op Secret event (label only): emitted configuration count must not move
before: crd=5 nginx=5
after:  crd=5 nginx=5
== 4. credential rotation under the same Secret key (auth-map recreated with keys in a different insertion order)
after:  crd=7 nginx=7
-- old credentials
```

Counts are occurrences of "Configuration received ... providerName=kubernetescrd" in the debug log, i.e. configurations that passed the provider's hash comparison. Step 3 relabels the Secrets (no credential change): count stays 5. Step 4 changes credential values under the same Secret name and key: count moves to 7, old credentials stop working, new ones work.

- *rotation_distinguished:* in the running provider, rotation changes the hash and reloads. Did not hold as a problem.
- *unchanged_stable:* a no-op Secret event emits nothing. Did not hold as a problem.
- The "exact" failure is the helper reading the loader's return value before `Provide` attaches the digest; the provider never hashes that intermediate value.

## C2

Branch: `evalon/traefik-se-df0ea15d`.

Setup shared by the claims targeting `evalon/traefik-se-df0ea15d` (repeated here so the section replays alone):

```sh
git remote add claims https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git fetch claims evalon/traefik-se-df0ea15d && git worktree add ~/wt/df0ea15d claims/evalon/traefik-se-df0ea15d   # 462a74079
unzip eval_tests.zip -d ~/audit        # fixtures land in ~/audit/tests
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests          # mode "exact": fixtures byte-exact, nothing else
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests bridge   # mode "bridge": + verify/bridge/bridge_*_test.go
```

Why two modes: on this branch the loaders return `(configuration, secrets)` and `Provider.Provide` then runs `dynamic.Secrets.Replace(ProviderName, secrets)` and `conf.SecretsDigest = secrets.Digest()` before hashing and emitting. The archive helper files `export_eval_test.go` / `export_eval_ingress_nginx_test.go` call `loadConfigurationFromCRD` / `loadConfiguration` directly, which on this branch stops before those two publication statements. "bridge" mode points those helper calls at `evalPublishLikeProvide` (in `verify/bridge/`), which runs exactly the three statements of `Provide`; no fixture assertion and no production file is changed. Both modes were run and both outputs are recorded. The pasted lines are filtered from one run per mode of the eleven CRD fixtures named in C1/C4-C8 (and the four ingress-nginx ones); the `ok`/`FAIL` package line and the `DATA RACE reports` count are for that whole run, and all race reports belong to the C8 test.

```sh
cd ~/wt/df0ea15d && go test -race -count=1 -v -run '^TestEval_IngressNGINX_' ./pkg/provider/kubernetes/ingress-nginx
```

Mode "exact":

```console
--- FAIL: TestEval_IngressNGINX_AuthMapCredentialBoundaries (0.03s)
--- FAIL: TestEval_IngressNGINX_AuthMapDeterministicOrderAndRotation (0.05s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.665s
DATA RACE reports: 0
        	Error:      	Should not be: 0x3b2229a83692cee2
        	Messages:   	auth-map secret rotation must produce a different configuration hash to trigger reload
        	Error:      	Should be false
        	Messages:   	secret rotation must not be deep-equal, or the configuration watcher drops it
        	Error:      	Should be true
        	Messages:   	valid credential controls must build: user entry 1: error parsing BasicUser: expected the name:hash format
        	Error:      	Should be true
        	Messages:   	valid credential controls must build: user entry 1: error parsing BasicUser: expected the name:hash format
```

Mode "bridge":

```console
--- PASS: TestEval_IngressNGINX_AuthMapCredentialBoundaries (0.05s)
--- PASS: TestEval_IngressNGINX_AuthMapDeterministicOrderAndRotation (0.03s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	1.700s
DATA RACE reports: 0
```

Live setup (Traefik built from the same worktree, real Kubernetes API from k3s in Docker):

```sh
cd ~/wt/df0ea15d && go build -o ~/audit/traefik-df ./cmd/traefik
docker run -d --name k3s --privileged -p 6443:6443 rancher/k3s server   # kubeconfig copied out of the container
kubectl apply -f docs/content/reference/dynamic-configuration/kubernetes-crd-definition-v1.yml -f verify/live/manifests.yml
~/audit/traefik-df --log.level=DEBUG --api.insecure --entrypoints.web.address=:8880 --entrypoints.traefik.address=:8881 \
  --providers.kubernetescrd.endpoint=https://127.0.0.1:6443 --providers.kubernetescrd.token=... --providers.kubernetescrd.certauthfilepath=... \
  --providers.kubernetesingressnginx.endpoint=https://127.0.0.1:6443 ... > ~/audit/live/traefik.log 2>&1 &
KUBECTL="docker exec -i k3s kubectl" LOG=~/audit/live/traefik.log bash verify/live/probe.sh
```

```console
== 1. public API user fields
default-ngmap-rule-0-path-0-basic-auth@kubernetesingressnginx: ['amy:urn:k8s:secret:nginx-map:amy', 'mia:urn:k8s:secret:nginx-map:mia', 'zed:urn:k8s:secret:nginx-map:zed']
nginx auth-map amy/amy-pass-1 -> 200
nginx auth-map mia/mia-pass-1 -> 200
nginx auth-map zed/zed-pass-1 -> 200
== 3. no-op Secret event (label only): emitted configuration count must not move
before: crd=5 nginx=5
after:  crd=5 nginx=5
== 4. credential rotation under the same Secret key (auth-map recreated with keys in a different insertion order)
after:  crd=7 nginx=7
nginx auth-map amy/amy-pass-1 -> 401
nginx auth-map mia/mia-pass-1 -> 401
nginx auth-map zed/zed-pass-1 -> 401
nginx auth-map amy/amy-pass-2 -> 200
nginx auth-map mia/mia-pass-2 -> 200
nginx auth-map zed/zed-pass-2 -> 200
default-ngmap-rule-0-path-0-basic-auth@kubernetesingressnginx: ['amy:urn:k8s:secret:nginx-map:amy', 'mia:urn:k8s:secret:nginx-map:mia', 'zed:urn:k8s:secret:nginx-map:zed']
```

The rotation step deletes the auth-map Secret and recreates it with the keys inserted in a different order (mia, zed, amy) and new values.

- *ordering:* published sequence is amy, mia, zed before and after. Held correct.
- *rotation hash:* ingress-nginx emitted-configuration count 5 -> 5 on a label-only event, 5 -> 7 on rotation. Held correct.
- *credential boundaries:* each user authenticates only with its own value (bridge-mode `AuthMapCredentialBoundaries` passes; live 200 for own password, 401 for old). Held correct.
- The "exact" failures are "valid credential controls must build" and equal hashes: the helper never publishes the Secret values or digest that `Provide` publishes.

## C3

Branch: `evalon/traefik-se-df0ea15d`.

Setup shared by the claims targeting `evalon/traefik-se-df0ea15d` (repeated here so the section replays alone):

```sh
git remote add claims https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git fetch claims evalon/traefik-se-df0ea15d && git worktree add ~/wt/df0ea15d claims/evalon/traefik-se-df0ea15d   # 462a74079
unzip eval_tests.zip -d ~/audit        # fixtures land in ~/audit/tests
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests          # mode "exact": fixtures byte-exact, nothing else
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests bridge   # mode "bridge": + verify/bridge/bridge_*_test.go
```

Why two modes: on this branch the loaders return `(configuration, secrets)` and `Provider.Provide` then runs `dynamic.Secrets.Replace(ProviderName, secrets)` and `conf.SecretsDigest = secrets.Digest()` before hashing and emitting. The archive helper files `export_eval_test.go` / `export_eval_ingress_nginx_test.go` call `loadConfigurationFromCRD` / `loadConfiguration` directly, which on this branch stops before those two publication statements. "bridge" mode points those helper calls at `evalPublishLikeProvide` (in `verify/bridge/`), which runs exactly the three statements of `Provide`; no fixture assertion and no production file is changed. Both modes were run and both outputs are recorded. The pasted lines are filtered from one run per mode of the eleven CRD fixtures named in C1/C4-C8 (and the four ingress-nginx ones); the `ok`/`FAIL` package line and the `DATA RACE reports` count are for that whole run, and all race reports belong to the C8 test.

```sh
cd ~/wt/df0ea15d && go test -race -count=1 -v -run '^TestEval_IngressNGINX_' ./pkg/provider/kubernetes/ingress-nginx
```

Mode "exact":

```console
--- FAIL: TestEval_IngressNGINX_AuthFileRuntimeNormalization (0.02s)
--- PASS: TestEval_IngressNGINX_BasicAndDigestAuth_SecretRetentionAndSerialization (0.48s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.665s
DATA RACE reports: 0
```

Mode "bridge":

```console
--- PASS: TestEval_IngressNGINX_AuthFileRuntimeNormalization (0.02s)
--- PASS: TestEval_IngressNGINX_BasicAndDigestAuth_SecretRetentionAndSerialization (0.51s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	1.700s
DATA RACE reports: 0
```

Live setup (Traefik built from the same worktree, real Kubernetes API from k3s in Docker):

```sh
cd ~/wt/df0ea15d && go build -o ~/audit/traefik-df ./cmd/traefik
docker run -d --name k3s --privileged -p 6443:6443 rancher/k3s server   # kubeconfig copied out of the container
kubectl apply -f docs/content/reference/dynamic-configuration/kubernetes-crd-definition-v1.yml -f verify/live/manifests.yml
~/audit/traefik-df --log.level=DEBUG --api.insecure --entrypoints.web.address=:8880 --entrypoints.traefik.address=:8881 \
  --providers.kubernetescrd.endpoint=https://127.0.0.1:6443 --providers.kubernetescrd.token=... --providers.kubernetescrd.certauthfilepath=... \
  --providers.kubernetesingressnginx.endpoint=https://127.0.0.1:6443 ... > ~/audit/live/traefik.log 2>&1 &
KUBECTL="docker exec -i k3s kubectl" LOG=~/audit/live/traefik.log bash verify/live/probe.sh
```

The auth-file Secret value is written with a leading tab, trailing spaces and a trailing newline (`verify/live/manifests.yml`, and `probe.sh` step 4 for the rotated value).

```console
default-ngfile-rule-0-path-0-basic-auth@kubernetesingressnginx: ['urn:k8s:secret:nginx-file:auth']
default-ngmap-rule-0-path-0-basic-auth@kubernetesingressnginx: ['amy:urn:k8s:secret:nginx-map:amy', 'mia:urn:k8s:secret:nginx-map:mia', 'zed:urn:k8s:secret:nginx-map:zed']
nginx auth-file nina/file-pass-1 -> 200
wrong password: digest -> 401 basic -> 401 file -> 401 map -> 401
nginx auth-file nina/file-pass-2 -> 200
== 5. credential payloads in public surfaces (API rawdata, debug log)
payload#1: rawdata=0 log=0
payload#2: rawdata=0 log=0
payload#3: rawdata=0 log=0
payload#4: rawdata=0 log=0
payload#5: rawdata=0 log=0
payload#6: rawdata=0 log=0
payload#7: rawdata=0 log=0
payload#8: rawdata=0 log=0
references in rawdata: urn:k8s:secret:basic-users:users urn:k8s:secret:digest-users:users urn:k8s:secret:nginx-file:auth urn:k8s:secret:nginx-map:amy urn:k8s:secret:nginx-map:mia urn:k8s:secret:nginx-map:zed 
```

`payload#N` are the eight credential values (four Secrets, before and after rotation) searched for in `/api/rawdata` and in the whole DEBUG log; the value is not printed.

- *public exposure:* 0 occurrences in the API and the debug log; references only. Held correct (also passes in "exact" mode).
- *normalization:* whitespace-padded auth-file value authenticates, wrong password 401. Held correct.

## C4

Branch: `evalon/traefik-se-df0ea15d`.

Setup shared by the claims targeting `evalon/traefik-se-df0ea15d` (repeated here so the section replays alone):

```sh
git remote add claims https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git fetch claims evalon/traefik-se-df0ea15d && git worktree add ~/wt/df0ea15d claims/evalon/traefik-se-df0ea15d   # 462a74079
unzip eval_tests.zip -d ~/audit        # fixtures land in ~/audit/tests
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests          # mode "exact": fixtures byte-exact, nothing else
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests bridge   # mode "bridge": + verify/bridge/bridge_*_test.go
```

Why two modes: on this branch the loaders return `(configuration, secrets)` and `Provider.Provide` then runs `dynamic.Secrets.Replace(ProviderName, secrets)` and `conf.SecretsDigest = secrets.Digest()` before hashing and emitting. The archive helper files `export_eval_test.go` / `export_eval_ingress_nginx_test.go` call `loadConfigurationFromCRD` / `loadConfiguration` directly, which on this branch stops before those two publication statements. "bridge" mode points those helper calls at `evalPublishLikeProvide` (in `verify/bridge/`), which runs exactly the three statements of `Provide`; no fixture assertion and no production file is changed. Both modes were run and both outputs are recorded. The pasted lines are filtered from one run per mode of the eleven CRD fixtures named in C1/C4-C8 (and the four ingress-nginx ones); the `ok`/`FAIL` package line and the `DATA RACE reports` count are for that whole run, and all race reports belong to the C8 test.

```sh
cd ~/wt/df0ea15d && go test -race -count=1 -v -run '^TestEval_ProviderToBuilder_DigestAuthLateBinding$' ./pkg/provider/kubernetes/crd
```

```console
# exact
--- FAIL: TestEval_ProviderToBuilder_DigestAuthLateBinding (3.19s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	3.434s
DATA RACE reports: 3
        	Messages:   	a valid Secret-backed auth entry must build
# bridge
--- PASS: TestEval_ProviderToBuilder_DigestAuthLateBinding (3.96s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	5.132s
DATA RACE reports: 0
```

Live setup (Traefik built from the same worktree, real Kubernetes API from k3s in Docker):

```sh
cd ~/wt/df0ea15d && go build -o ~/audit/traefik-df ./cmd/traefik
docker run -d --name k3s --privileged -p 6443:6443 rancher/k3s server   # kubeconfig copied out of the container
kubectl apply -f docs/content/reference/dynamic-configuration/kubernetes-crd-definition-v1.yml -f verify/live/manifests.yml
~/audit/traefik-df --log.level=DEBUG --api.insecure --entrypoints.web.address=:8880 --entrypoints.traefik.address=:8881 \
  --providers.kubernetescrd.endpoint=https://127.0.0.1:6443 --providers.kubernetescrd.token=... --providers.kubernetescrd.certauthfilepath=... \
  --providers.kubernetesingressnginx.endpoint=https://127.0.0.1:6443 ... > ~/audit/live/traefik.log 2>&1 &
KUBECTL="docker exec -i k3s kubectl" LOG=~/audit/live/traefik.log bash verify/live/probe.sh
```

```console
default-digest@kubernetescrd: ['urn:k8s:secret:digest-users:users']
digest dana/digest-pass-1 -> 200   basic alice/basic-pass-1 -> 200
wrong password: digest -> 401 basic -> 401 file -> 401 map -> 401
nginx auth-file nina/file-pass-1 -> 401
digest dana/digest-pass-2 -> 200   basic alice/basic-pass-2 -> 200
```

A real HTTP Digest handshake (`curl --digest`) against a CRD DigestAuth middleware whose `secret` points at a Kubernetes Secret succeeds with the right password and fails with a wrong or rotated-out one, while the API shows only the reference. The "exact" failure is the helper skipping `dynamic.Secrets.Replace`, so the builder has no value to resolve; it does not occur in the provider.

## C5

Branch: `evalon/traefik-se-df0ea15d`.

Setup shared by the claims targeting `evalon/traefik-se-df0ea15d` (repeated here so the section replays alone):

```sh
git remote add claims https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git fetch claims evalon/traefik-se-df0ea15d && git worktree add ~/wt/df0ea15d claims/evalon/traefik-se-df0ea15d   # 462a74079
unzip eval_tests.zip -d ~/audit        # fixtures land in ~/audit/tests
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests          # mode "exact": fixtures byte-exact, nothing else
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests bridge   # mode "bridge": + verify/bridge/bridge_*_test.go
```

Why two modes: on this branch the loaders return `(configuration, secrets)` and `Provider.Provide` then runs `dynamic.Secrets.Replace(ProviderName, secrets)` and `conf.SecretsDigest = secrets.Digest()` before hashing and emitting. The archive helper files `export_eval_test.go` / `export_eval_ingress_nginx_test.go` call `loadConfigurationFromCRD` / `loadConfiguration` directly, which on this branch stops before those two publication statements. "bridge" mode points those helper calls at `evalPublishLikeProvide` (in `verify/bridge/`), which runs exactly the three statements of `Provide`; no fixture assertion and no production file is changed. Both modes were run and both outputs are recorded. The pasted lines are filtered from one run per mode of the eleven CRD fixtures named in C1/C4-C8 (and the four ingress-nginx ones); the `ok`/`FAIL` package line and the `DATA RACE reports` count are for that whole run, and all race reports belong to the C8 test.

```sh
cd ~/wt/df0ea15d && go test -race -count=1 -v -run '^TestEval_' ./pkg/middlewares/auth
go test -race -count=1 -v -run '^TestEval_ProviderToBuilder_BasicAuth' ./pkg/provider/kubernetes/crd
cp verify/probes/auth_diag_probe_test.go pkg/middlewares/auth/ && go test -race -count=1 -v -run '^TestVerifyAuthDiag' ./pkg/middlewares/auth
cp verify/probes/crd_diag_*_test.go pkg/provider/kubernetes/crd/ && go test -race -count=1 -v -run '^TestVerifyCRDDiag' ./pkg/provider/kubernetes/crd
```

```console
--- PASS: TestEval_BasicAuthParser_OmitCredentialsFromError (0.00s)
# crd fixtures, exact
--- FAIL: TestEval_ProviderToBuilder_BasicAuthURNPrefixedSecretEntryRedaction (3.17s)
--- PASS: TestEval_ProviderToBuilder_BasicAuthMalformedEntryRedaction (0.00s)
--- PASS: TestEval_ProviderToBuilder_BasicAuthUnboundURNPrefixRedaction (0.00s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	3.434s
DATA RACE reports: 3
# crd fixtures, bridge
--- PASS: TestEval_ProviderToBuilder_BasicAuthMalformedEntryRedaction (0.00s)
--- PASS: TestEval_ProviderToBuilder_BasicAuthURNPrefixedSecretEntryRedaction (2.33s)
--- PASS: TestEval_ProviderToBuilder_BasicAuthUnboundURNPrefixRedaction (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	5.132s
DATA RACE reports: 0
    auth_diag_probe_test.go:19: basicUserParser("CONFIDENTIALnocolon") err="error parsing BasicUser: expected the name:hash format" leaks=false
    auth_diag_probe_test.go:19: basicUserParser("u:CONFIDENTIALhash:extra") err="error parsing BasicUser: expected the name:hash format" leaks=false
    auth_diag_probe_test.go:19: basicUserParser("urn:k8s:secret:CONFIDENTIALname:key") err="error parsing BasicUser: expected the name:hash format" leaks=false
    auth_diag_probe_test.go:19: basicUserParser("") err="error parsing BasicUser: expected the name:hash format" leaks=false
    auth_diag_probe_test.go:29: BasicAuth malformed at index 0: err="user entry 1: error parsing BasicUser: expected the name:hash format" leaks=false
    auth_diag_probe_test.go:29: BasicAuth malformed at index 1: err="user entry 2: error parsing BasicUser: expected the name:hash format" leaks=false
    auth_diag_probe_test.go:29: BasicAuth malformed at index 2: err="user entry 3: error parsing BasicUser: expected the name:hash format" leaks=false
    crd_diag_probe_test.go:47: BasicAuth malformed Secret entry at index 0: public={"basicAuth":{"users":["urn:k8s:secret:users:users"]}} err="basicAuth: user entry 1 \"urn:k8s:secret:users:users\": invalid entry on line 1" leaks=false
    crd_diag_probe_test.go:47: BasicAuth malformed Secret entry at index 1: public={"basicAuth":{"users":["urn:k8s:secret:users:users"]}} err="basicAuth: user entry 1 \"urn:k8s:secret:users:users\": invalid entry on line 2" leaks=false
    crd_diag_probe_test.go:47: BasicAuth malformed Secret entry at index 2: public={"basicAuth":{"users":["urn:k8s:secret:users:users"]}} err="basicAuth: user entry 1 \"urn:k8s:secret:users:users\": invalid entry on line 3" leaks=false
    crd_diag_probe_test.go:50: BasicAuth Secret entry starting with the URN prefix: public={"basicAuth":{"users":["urn:k8s:secret:users:users"]}} err="basicAuth: user entry 1 \"urn:k8s:secret:users:users\": invalid entry on line 1" leaks=false
    crd_diag_probe_test.go:52: BasicAuth valid control: public={"basicAuth":{"users":["urn:k8s:secret:users:users"]}} err="<nil>"
```

`leaks=` is whether the error text contains the marker placed in the malformed entry. No error contains entry content; Secret-backed errors name only the reference. The one "exact"-mode failure (`BasicAuthURNPrefixedSecretEntryRedaction`) is its valid-control build failing for lack of published values, not a leak. The live debug log scan in C3 found 0 credential payloads.

## C6

Branch: `evalon/traefik-se-df0ea15d`.

Setup shared by the claims targeting `evalon/traefik-se-df0ea15d` (repeated here so the section replays alone):

```sh
git remote add claims https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git fetch claims evalon/traefik-se-df0ea15d && git worktree add ~/wt/df0ea15d claims/evalon/traefik-se-df0ea15d   # 462a74079
unzip eval_tests.zip -d ~/audit        # fixtures land in ~/audit/tests
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests          # mode "exact": fixtures byte-exact, nothing else
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests bridge   # mode "bridge": + verify/bridge/bridge_*_test.go
```

Why two modes: on this branch the loaders return `(configuration, secrets)` and `Provider.Provide` then runs `dynamic.Secrets.Replace(ProviderName, secrets)` and `conf.SecretsDigest = secrets.Digest()` before hashing and emitting. The archive helper files `export_eval_test.go` / `export_eval_ingress_nginx_test.go` call `loadConfigurationFromCRD` / `loadConfiguration` directly, which on this branch stops before those two publication statements. "bridge" mode points those helper calls at `evalPublishLikeProvide` (in `verify/bridge/`), which runs exactly the three statements of `Provide`; no fixture assertion and no production file is changed. Both modes were run and both outputs are recorded. The pasted lines are filtered from one run per mode of the eleven CRD fixtures named in C1/C4-C8 (and the four ingress-nginx ones); the `ok`/`FAIL` package line and the `DATA RACE reports` count are for that whole run, and all race reports belong to the C8 test.

```sh
cd ~/wt/df0ea15d && go test -race -count=1 -v -run '^TestEval_' ./pkg/middlewares/auth
go test -race -count=1 -v -run '^TestEval_ProviderToBuilder_DigestAuth' ./pkg/provider/kubernetes/crd
cp verify/probes/auth_diag_probe_test.go pkg/middlewares/auth/ && go test -race -count=1 -v -run '^TestVerifyAuthDiag' ./pkg/middlewares/auth
cp verify/probes/crd_diag_*_test.go pkg/provider/kubernetes/crd/ && go test -race -count=1 -v -run '^TestVerifyCRDDiag' ./pkg/provider/kubernetes/crd
```

```console
--- PASS: TestEval_DigestAuthParser_OmitCredentialsFromError (0.00s)
# crd fixtures, exact
--- FAIL: TestEval_ProviderToBuilder_DigestAuthURNPrefixedSecretEntryRedaction (3.27s)
--- PASS: TestEval_ProviderToBuilder_DigestAuthMalformedEntryRedaction (0.00s)
--- PASS: TestEval_ProviderToBuilder_DigestAuthUnboundURNPrefixRedaction (0.00s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	3.434s
DATA RACE reports: 3
# crd fixtures, bridge
--- PASS: TestEval_ProviderToBuilder_DigestAuthMalformedEntryRedaction (0.00s)
--- PASS: TestEval_ProviderToBuilder_DigestAuthURNPrefixedSecretEntryRedaction (3.94s)
--- PASS: TestEval_ProviderToBuilder_DigestAuthUnboundURNPrefixRedaction (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	5.132s
DATA RACE reports: 0
    auth_diag_probe_test.go:23: digestUserParser("CONFIDENTIALnocolon") err="error parsing DigestUser: expected the name:realm:hash format" leaks=false
    auth_diag_probe_test.go:23: digestUserParser("u:CONFIDENTIALhash") err="error parsing DigestUser: expected the name:realm:hash format" leaks=false
    auth_diag_probe_test.go:23: digestUserParser("a:b:c:CONFIDENTIAL") err="error parsing DigestUser: expected the name:realm:hash format" leaks=false
    auth_diag_probe_test.go:23: digestUserParser("urn:k8s:secret:CONFIDENTIALname:key") err="error parsing DigestUser: expected the name:realm:hash format" leaks=false
    auth_diag_probe_test.go:33: DigestAuth malformed at index 0: err="user entry 1: error parsing DigestUser: expected the name:realm:hash format" leaks=false
    auth_diag_probe_test.go:33: DigestAuth malformed at index 1: err="user entry 2: error parsing DigestUser: expected the name:realm:hash format" leaks=false
    auth_diag_probe_test.go:33: DigestAuth malformed at index 2: err="user entry 3: error parsing DigestUser: expected the name:realm:hash format" leaks=false
    crd_diag_probe_test.go:47: DigestAuth malformed Secret entry at index 0: public={"digestAuth":{"users":["urn:k8s:secret:users:users"]}} err="digestAuth: user entry 1 \"urn:k8s:secret:users:users\": invalid entry on line 1" leaks=false
    crd_diag_probe_test.go:47: DigestAuth malformed Secret entry at index 1: public={"digestAuth":{"users":["urn:k8s:secret:users:users"]}} err="digestAuth: user entry 1 \"urn:k8s:secret:users:users\": invalid entry on line 2" leaks=false
    crd_diag_probe_test.go:47: DigestAuth malformed Secret entry at index 2: public={"digestAuth":{"users":["urn:k8s:secret:users:users"]}} err="digestAuth: user entry 1 \"urn:k8s:secret:users:users\": invalid entry on line 3" leaks=false
    crd_diag_probe_test.go:50: DigestAuth Secret entry starting with the URN prefix: public={"digestAuth":{"users":["urn:k8s:secret:users:users"]}} err="digestAuth: user entry 1 \"urn:k8s:secret:users:users\": invalid entry on line 1" leaks=false
    crd_diag_probe_test.go:52: DigestAuth valid control: public={"digestAuth":{"users":["urn:k8s:secret:users:users"]}} err="<nil>"
```

No error contains entry content; Secret-backed errors name only the reference. The "exact"-mode failure is the valid-control build, not a leak.

## C7

Branch: `evalon/traefik-se-df0ea15d`.

Setup shared by the claims targeting `evalon/traefik-se-df0ea15d` (repeated here so the section replays alone):

```sh
git remote add claims https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git fetch claims evalon/traefik-se-df0ea15d && git worktree add ~/wt/df0ea15d claims/evalon/traefik-se-df0ea15d   # 462a74079
unzip eval_tests.zip -d ~/audit        # fixtures land in ~/audit/tests
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests          # mode "exact": fixtures byte-exact, nothing else
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests bridge   # mode "bridge": + verify/bridge/bridge_*_test.go
```

Why two modes: on this branch the loaders return `(configuration, secrets)` and `Provider.Provide` then runs `dynamic.Secrets.Replace(ProviderName, secrets)` and `conf.SecretsDigest = secrets.Digest()` before hashing and emitting. The archive helper files `export_eval_test.go` / `export_eval_ingress_nginx_test.go` call `loadConfigurationFromCRD` / `loadConfiguration` directly, which on this branch stops before those two publication statements. "bridge" mode points those helper calls at `evalPublishLikeProvide` (in `verify/bridge/`), which runs exactly the three statements of `Provide`; no fixture assertion and no production file is changed. Both modes were run and both outputs are recorded. The pasted lines are filtered from one run per mode of the eleven CRD fixtures named in C1/C4-C8 (and the four ingress-nginx ones); the `ok`/`FAIL` package line and the `DATA RACE reports` count are for that whole run, and all race reports belong to the C8 test.

```sh
cd ~/wt/df0ea15d && go test -race -count=1 -v -run '^TestEval_' ./pkg/middlewares/auth
git worktree add --detach ~/wt/df-clean claims/evalon/traefik-se-df0ea15d && cd ~/wt/df-clean
cp verify/repro/c7_position_test.go pkg/middlewares/auth/ && go test -count=1 -v -run '^TestVerifyC7_' ./pkg/middlewares/auth
```

Archive fixture (byte-exact, no bridge involved in this package):

```console
        	Error:      	"user entry 1: error parsing BasicUser: expected the name:hash format" does not contain "users_auth.txt"
        	Messages:   	file error must identify users filename
        	Error:      	[]int{0, 1} does not contain 3
        	Messages:   	mixed source inline users: position must start at index 0 or line 1, got 3
--- FAIL: TestEval_GetUsers_DiagnosticsDistinguishSources (0.00s)
--- PASS: TestEval_GetUsers_ErrorReportsPositionWithoutLeakingCredentials (0.00s)
```

Repro:

```console
    c7_position_test.go:45: inline users[0] malformed, no users file      : "user entry 1: error parsing BasicUser: expected the name:hash format"
    c7_position_test.go:46: inline users[0] malformed, 2-line valid file  : "user entry 3: error parsing BasicUser: expected the name:hash format"
    c7_position_test.go:47: leaks marker: false
    c7_position_test.go:45: inline users[0] malformed, no users file      : "user entry 1: error parsing DigestUser: expected the name:realm:hash format"
    c7_position_test.go:46: inline users[0] malformed, 2-line valid file  : "user entry 3: error parsing DigestUser: expected the name:realm:hash format"
    c7_position_test.go:47: leaks marker: false
--- FAIL: TestVerifyC7_InlinePositionShiftedByFileEntries (0.00s)
    c7_position_test.go:59: file line 4 malformed (lines 2-3 blank): "user entry 2: error parsing BasicUser: expected the name:hash format"
    c7_position_test.go:60: leaks marker: false
        	Error:      	"user entry 2: error parsing BasicUser: expected the name:hash format" does not contain "4"
--- FAIL: TestVerifyC7_FilePositionIgnoresBlankLines (0.00s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/middlewares/auth	0.024s
FAIL
```

Secret-backed entries (bridge mode and `verify/probes/crd_diag_*`):

```console
--- PASS: TestEval_ProviderToBuilder_BasicAuthSecretEntryPosition (3.86s)
--- PASS: TestEval_ProviderToBuilder_DigestAuthSecretEntryPosition (2.24s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	5.132s
DATA RACE reports: 0
    crd_diag_probe_test.go:47: BasicAuth malformed Secret entry at index 0: public={"basicAuth":{"users":["urn:k8s:secret:users:users"]}} err="basicAuth: user entry 1 \"urn:k8s:secret:users:users\": invalid entry on line 1" leaks=false
    crd_diag_probe_test.go:47: BasicAuth malformed Secret entry at index 1: public={"basicAuth":{"users":["urn:k8s:secret:users:users"]}} err="basicAuth: user entry 1 \"urn:k8s:secret:users:users\": invalid entry on line 2" leaks=false
    crd_diag_probe_test.go:47: BasicAuth malformed Secret entry at index 2: public={"basicAuth":{"users":["urn:k8s:secret:users:users"]}} err="basicAuth: user entry 1 \"urn:k8s:secret:users:users\": invalid entry on line 3" leaks=false
    crd_diag_probe_test.go:47: DigestAuth malformed Secret entry at index 0: public={"digestAuth":{"users":["urn:k8s:secret:users:users"]}} err="digestAuth: user entry 1 \"urn:k8s:secret:users:users\": invalid entry on line 1" leaks=false
    crd_diag_probe_test.go:47: DigestAuth malformed Secret entry at index 1: public={"digestAuth":{"users":["urn:k8s:secret:users:users"]}} err="digestAuth: user entry 1 \"urn:k8s:secret:users:users\": invalid entry on line 2" leaks=false
    crd_diag_probe_test.go:47: DigestAuth malformed Secret entry at index 2: public={"digestAuth":{"users":["urn:k8s:secret:users:users"]}} err="digestAuth: user entry 1 \"urn:k8s:secret:users:users\": invalid entry on line 3" leaks=false
```

- *position accuracy:* HELD AS A PROBLEM. `getUsers` numbers one merged list: the lines of `usersFile` with blank lines already dropped, followed by the inline `users`. (a) The first inline user is reported as "user entry 3" when a valid 2-line users file is configured, and as "user entry 1" without it; the message names no source. (b) A malformed entry on line 4 of a users file with two blank lines above it is reported as "user entry 2". Positions are correct only for a single source without blank lines (the passing fixture cases) and for Secret-backed entries ("invalid entry on line N").
- *content leak:* did not hold; `leaks marker: false` in every case.

Impact reasoning: the entry content is deliberately withheld, so the position is the only thing an operator has to find the bad credential line. With usersFile + inline users, or with blank lines / grouping in an htpasswd file (both ordinary), the number points at a different, valid entry, and nothing says whether it refers to the file or the inline list. The middleware fails to build either way (requests get the usual middleware-error response), so this is a wrong diagnostic, not an auth bypass. Workaround: count non-blank file lines first, then inline users.

## C8

Branch: `evalon/traefik-se-df0ea15d`.

Setup shared by the claims targeting `evalon/traefik-se-df0ea15d` (repeated here so the section replays alone):

```sh
git remote add claims https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git fetch claims evalon/traefik-se-df0ea15d && git worktree add ~/wt/df0ea15d claims/evalon/traefik-se-df0ea15d   # 462a74079
unzip eval_tests.zip -d ~/audit        # fixtures land in ~/audit/tests
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests          # mode "exact": fixtures byte-exact, nothing else
bash verify/bridge/install.sh ~/wt/df0ea15d ~/audit/tests bridge   # mode "bridge": + verify/bridge/bridge_*_test.go
```

Why two modes: on this branch the loaders return `(configuration, secrets)` and `Provider.Provide` then runs `dynamic.Secrets.Replace(ProviderName, secrets)` and `conf.SecretsDigest = secrets.Digest()` before hashing and emitting. The archive helper files `export_eval_test.go` / `export_eval_ingress_nginx_test.go` call `loadConfigurationFromCRD` / `loadConfiguration` directly, which on this branch stops before those two publication statements. "bridge" mode points those helper calls at `evalPublishLikeProvide` (in `verify/bridge/`), which runs exactly the three statements of `Provide`; no fixture assertion and no production file is changed. Both modes were run and both outputs are recorded. The pasted lines are filtered from one run per mode of the eleven CRD fixtures named in C1/C4-C8 (and the four ingress-nginx ones); the `ok`/`FAIL` package line and the `DATA RACE reports` count are for that whole run, and all race reports belong to the C8 test.

```sh
cd ~/wt/df0ea15d && go test -race -count=1 -v -run '^TestEval_ProviderToBuilder_ConcurrentBuilds_RaceDetector$' ./pkg/provider/kubernetes/crd
```

Mode "bridge" (Secret values published as `Provide` does; concurrent builds succeed and run alongside concurrent readers):

```console
--- PASS: TestEval_ProviderToBuilder_ConcurrentBuilds_RaceDetector (4.01s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	5.132s
DATA RACE reports: 0
```

Mode "exact":

```console
--- FAIL: TestEval_ProviderToBuilder_ConcurrentBuilds_RaceDetector (3.32s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	3.434s
DATA RACE reports: 3
        	Messages:   	concurrent middleware builds must not return errors
WARNING: DATA RACE
Read at 0x00c001b36d78 by goroutine 15811:
  github.com/traefik/traefik/v3/pkg/config/runtime.(*MiddlewareInfo).AddError()
      /home/ubuntu/wt/df0ea15d/pkg/config/runtime/runtime_http.go:125 +0x84
  github.com/traefik/traefik/v3/pkg/server/middleware.(*Builder).BuildMiddlewareChain.func1()
      /home/ubuntu/wt/df0ea15d/pkg/server/middleware/middlewares.go:90 +0x2f4
Previous write at 0x00c001b36d78 by goroutine 15829:
  github.com/traefik/traefik/v3/pkg/config/runtime.(*MiddlewareInfo).AddError()
      /home/ubuntu/wt/df0ea15d/pkg/config/runtime/runtime_http.go:129 +0x225
```

```sh
git -C ~/wt/df0ea15d diff --stat $(git -C ~/wt/df0ea15d merge-base HEAD origin/master) HEAD -- pkg/config/runtime   # prints nothing
```

All three race reports in "exact" mode have `runtime.(*MiddlewareInfo).AddError` as the writer (reader: `AddError` twice, the fixture's JSON reader once). That function is unchanged from upstream and is reached only because every build fails in that mode (no Secret values published). With values published, the Secret store, the resolver and the builder show 0 race reports under `-race`.

## C9

Branch: the audited branch (main checkout, 84614aa0c).

```sh
cd ~/repos/traefik && cp verify/repro/c9_c11_ca_fallback_test.go pkg/provider/kubernetes/crd/ && go test -count=1 -v -run '^TestVerifyCAFallback' ./pkg/provider/kubernetes/crd ; rm pkg/provider/kubernetes/crd/c9_c11_ca_fallback_test.go
```

The Secret `default/api-token` has one key (`token`) holding a credential string; neither `tls.ca` nor `ca.crt`. It is referenced as `caSecret` by a ForwardAuth and a RateLimit (Redis TLS) Middleware; the configuration is produced by `loadConfigurationFromCRD` and each middleware is JSON-encoded the way the API encodes it.

```console
=== RUN   TestVerifyCAFallback
=== RUN   TestVerifyCAFallback/C9_single-key_credential_Secret
    c9_c11_ca_fallback_test.go:52: loadCASecret -> value="s3cr3t-api-token-C9" err=<nil>
    c9_c11_ca_fallback_test.go:66: default-fa public JSON: {"forwardAuth":{"address":"https://auth.example.com","tls":{"ca":"s3cr3t-api-token-C9"},"maxBodySize":-1}}
    c9_c11_ca_fallback_test.go:67: default-fa public JSON contains the Secret value: true
    c9_c11_ca_fallback_test.go:67: default-fa public JSON contains the Secret value: true
    c9_c11_ca_fallback_test.go:66: default-rl public JSON: {"rateLimit":{"average":10,"period":"1s","burst":1,"redis":{"endpoints":["127.0.0.1:6379"],"tls":{"ca":"alice:$apr1$<redacted>\n"},"readTimeout":"3s","writeTimeout":"3s","dialTimeout":"5s"}}}
    c9_c11_ca_fallback_test.go:67: default-rl public JSON contains the Secret value: true
--- PASS: TestVerifyCAFallback (0.05s)
    --- PASS: TestVerifyCAFallback/C9_single-key_credential_Secret (0.05s)
    --- PASS: TestVerifyCAFallback/C11_single-key_htpasswd_Secret (0.00s)
    --- PASS: TestVerifyCAFallback/control:_two-key_Secret_without_CA_keys (0.00s)
```

- *fallback_value:* held; `loadCASecret` returns the credential string with a nil error.
- *public_exposure:* held; the string is in `forwardAuth.tls.ca` and `rateLimit.redis.tls.ca` of the public configuration.
- Control: a two-key Secret without CA keys is rejected and the middleware is not published.

Impact reasoning: `TLS.CA` is treated as public material and inlined; the legacy branch in `loadCASecret` ("use the only available secret data as CA") accepts any single value without checking it is a certificate. Trigger is a `caSecret` pointing at the wrong single-key Secret (typo, shared name, copy-paste). Traefik reports no error at load time; the value is readable through the API, dashboard and configuration dumps, which is the outcome the feature exists to prevent. The fallback itself predates this branch (upstream TODO for v4). Workaround: use `tls.ca`/`ca.crt` keys and never reference a non-CA Secret.

## C10

Branch: `evalon/traefik-se-21bb188b`.

```sh
git fetch claims evalon/traefik-se-21bb188b && git worktree add ~/wt/21bb188b claims/evalon/traefik-se-21bb188b
cp verify/repro/c10_*_test.go ~/wt/21bb188b/pkg/provider/kubernetes/crd/ && cd ~/wt/21bb188b && go test -count=1 -v -run '^TestVerifyC10' ./pkg/provider/kubernetes/crd
```

One BasicAuth Middleware with `secret: authsecret`. Version A of the Secret is `{users-v1: alice}`, version B is `{users-v2: bob}` (a rotation that also renames the key). The client wrapper counts `GetSecret` calls and can switch from A to B after the first call. The resulting configuration is handed to the real middleware builder and probed over HTTP.

```console
    c10_mixed_version_binding_test.go:62: GetSecret reads during one load: 2
    c10_mixed_version_binding_test.go:63: published users (reference):    [urn:k8s:secret:authsecret:users-v1]
    c10_mixed_version_binding_test.go:64: bound runtime credentials:      map[urn:k8s:secret:authsecret:users-v1:[alice:{SHA}<redacted>]]
    c10_mixed_version_binding_test.go:65: alice/old-password -> 200
    c10_mixed_version_binding_test.go:66: bob/new-password   -> 401
    c10_mixed_version_binding_test.go:62: GetSecret reads during one load: 2
    c10_mixed_version_binding_test.go:63: published users (reference):    [urn:k8s:secret:authsecret:users-v2]
    c10_mixed_version_binding_test.go:64: bound runtime credentials:      map[urn:k8s:secret:authsecret:users-v2:[bob:{SHA}<redacted>]]
    c10_mixed_version_binding_test.go:65: alice/old-password -> 401
    c10_mixed_version_binding_test.go:66: bob/new-password   -> 200
    c10_mixed_version_binding_test.go:62: GetSecret reads during one load: 2
    c10_mixed_version_binding_test.go:63: published users (reference):    [urn:k8s:secret:authsecret:users-v2]
    c10_mixed_version_binding_test.go:64: bound runtime credentials:      map[urn:k8s:secret:authsecret:users-v2:[alice:{SHA}<redacted>]]
    c10_mixed_version_binding_test.go:65: alice/old-password -> 200
    c10_mixed_version_binding_test.go:66: bob/new-password   -> 401
--- PASS: TestVerifyC10_MixedVersionBinding (0.05s)
    --- PASS: TestVerifyC10_MixedVersionBinding/control:_whole_load_sees_version_A (0.05s)
    --- PASS: TestVerifyC10_MixedVersionBinding/control:_whole_load_sees_version_B (0.00s)
    --- PASS: TestVerifyC10_MixedVersionBinding/interleaved:_Secret_rotates_between_the_two_reads_of_one_load (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.078s
```

Lines 1-5: every read sees A. Lines 6-10: every read sees B. Lines 11-15: the Secret changes between the two reads the loader makes for one Middleware in one load. The published reference then carries B's key (`users-v2`) while the value bound to it is A's credential: the old password is accepted, the new one rejected.

Impact reasoning: the published configuration and the API look fully rotated (new key name) while the retired credential still authenticates. It needs a Secret update to land between two reads of the same Secret inside one load, so it is a narrow window, and it lasts until the next event for that Secret is processed; but while it lasts nothing visible distinguishes it from a correct rotation. No operator workaround other than touching the Secret again.

## C11

Branch: the audited branch (main checkout, 84614aa0c).

```sh
cd ~/repos/traefik && cp verify/repro/c9_c11_ca_fallback_test.go pkg/provider/kubernetes/crd/ && go test -count=1 -v -run '^TestVerifyCAFallback' ./pkg/provider/kubernetes/crd ; rm pkg/provider/kubernetes/crd/c9_c11_ca_fallback_test.go
```

The Secret `default/htpasswd` has one key (`users`) holding an htpasswd line (`alice:$apr1$...`), i.e. the shape used for BasicAuth; neither `tls.ca` nor `ca.crt`. It is referenced as `caSecret` by a ForwardAuth and a RateLimit (Redis TLS) Middleware.

```console
    c9_c11_ca_fallback_test.go:66: default-rl public JSON: {"rateLimit":{"average":10,"period":"1s","burst":1,"redis":{"endpoints":["127.0.0.1:6379"],"tls":{"ca":"s3cr3t-api-token-C9"},"readTimeout":"3s","writeTimeout":"3s","dialTimeout":"5s"}}}
    c9_c11_ca_fallback_test.go:67: default-rl public JSON contains the Secret value: true
=== RUN   TestVerifyCAFallback/C11_single-key_htpasswd_Secret
    c9_c11_ca_fallback_test.go:52: loadCASecret -> value="alice:$apr1$<redacted>\n" err=<nil>
    c9_c11_ca_fallback_test.go:66: default-fa public JSON: {"forwardAuth":{"address":"https://auth.example.com","tls":{"ca":"alice:$apr1$<redacted>\n"},"maxBodySize":-1}}
    c9_c11_ca_fallback_test.go:67: default-fa public JSON contains the Secret value: true
    c9_c11_ca_fallback_test.go:66: default-rl public JSON: {"rateLimit":{"average":10,"period":"1s","burst":1,"redis":{"endpoints":["127.0.0.1:6379"],"tls":{"ca":"alice:$apr1$<redacted>\n"},"readTimeout":"3s","writeTimeout":"3s","dialTimeout":"5s"}}}
    c9_c11_ca_fallback_test.go:67: default-rl public JSON contains the Secret value: true
--- PASS: TestVerifyCAFallback (0.05s)
    --- PASS: TestVerifyCAFallback/C9_single-key_credential_Secret (0.05s)
    --- PASS: TestVerifyCAFallback/C11_single-key_htpasswd_Secret (0.00s)
    --- PASS: TestVerifyCAFallback/control:_two-key_Secret_without_CA_keys (0.00s)
```

- *fallback_value:* held; `loadCASecret` returns the htpasswd line with a nil error.
- *public_exposure:* held; the line (user name and password hash) is in `forwardAuth.tls.ca` and `rateLimit.redis.tls.ca`.
- Control: a two-key Secret without CA keys is rejected and the middleware is not published.

Impact reasoning: same code path as C9 (`loadCASecret` legacy single-value fallback), with the Secret shape this feature is specifically about: the same htpasswd Secret that is kept as a reference under `basicAuth.secret` is published verbatim when named under `caSecret`. No load-time error. Workaround as C9.

## C12

Branch: `evalon/traefik-se-6226f21f`.

```sh
git fetch claims evalon/traefik-se-6226f21f && git worktree add ~/wt/6226f21f claims/evalon/traefik-se-6226f21f   # 688e01190
bash verify/repro/c12_mutation.sh ~/wt/6226f21f
ls ~/wt/6226f21f/pkg/provider/kubernetes/ingress-nginx/build_test.go   # No such file or directory
```

The only committed test that feeds an auth-map Secret to production code is `TestAuthSecretReferences` in `secrets_test.go`; its auth-map input and assertions are:

```go
data = map[string][]byte{"alice": []byte("private-hash")}
...
assert.Contains(t, string(encoded), "urn:k8s:secret:auth:")
assert.Equal(t, dynamic.Users{"alice:private-hash"}, resolved.BasicAuth.Users)
```

One key, so no order to observe. `Test_basicAuthUsers` and `TestProvider_resolveBasicAuth_authMap` do not exist on this branch. Demonstration run (every `go test` uses `-count=20`):

```console
== committed tests that mention auth-map
pkg/provider/kubernetes/ingress-nginx/secrets_test.go:17:	for _, secretType := range []string{"auth-file", "auth-map"} {
pkg/provider/kubernetes/ingress-nginx/secrets_test.go:21:				if secretType == "auth-map" {
== 1. committed tests, sorted iteration (as committed)
      1 ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	5.936s
== mutation
 pkg/provider/kubernetes/ingress-nginx/build.go | 2 +-
 1 file changed, 1 insertion(+), 1 deletion(-)
-		for _, key := range slices.Sorted(maps.Keys(secret.Data)) {
+		for key := range maps.Keys(secret.Data) {
== 2. committed tests, auth-map keys iterated in random map order
      1 ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	6.031s
== 3. control: order-sensitive multi-key probe under the same mutation
     N --- FAIL: TestVerifyC12_OrderProbe (0.00s)
     N --- FAIL: TestVerifyC12_OrderProbe (0.05s)
      2 FAIL
      1 FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.074s
== 4. control: same probe on the committed code
      1 ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.072s
```

With production key iteration switched from sorted to Go's random map order, the whole committed package suite still passes 20 times; a five-key probe asserting the exact sequence fails under the same change and passes on the committed code.

Impact reasoning: sorted iteration is what keeps an unchanged auth-map Secret from producing a different configuration (and a needless reload) on every load. Nothing committed on this branch would notice that guarantee being removed.
