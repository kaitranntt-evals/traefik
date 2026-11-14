#!/bin/bash
# Run: KUBECTL="docker exec -i k3s kubectl" LOG=<traefik debug log> bash verify/live/probe.sh  (Traefik built from the audited branch on 127.0.0.1:8880/8881, manifests.yml applied, backend on 127.0.0.1:9000)
K=${KUBECTL:-kubectl}; LOG=${LOG:-traefik.log}
sha(){ printf '%s' "$1" | openssl dgst -binary -sha1 | base64; }
dg(){ printf '%s:%s:%s' "$1" traefik "$2" | md5sum | cut -d' ' -f1; }
code(){ curl -s -o /dev/null -w '%{http_code}' "$@"; }
users(){ curl -s 127.0.0.1:8881/api/http/middlewares/$1 | python3 -c 'import json,sys; d=json.load(sys.stdin); print((d.get("basicAuth") or d.get("digestAuth"))["users"])'; }
emitted(){ sed "s/\x1b\[[0-9;]*m//g" "$LOG" | grep -c "Configuration received.*providerName=$1"; }
auth(){
  echo "digest dana/$1 -> $(code --digest -u dana:$1 127.0.0.1:8880/digest/)   basic alice/$2 -> $(code -u alice:$2 127.0.0.1:8880/basic/)"
  echo "nginx auth-file nina/$3 -> $(code -u nina:$3 -H 'Host: file.example.com' 127.0.0.1:8880/)"
  for u in amy mia zed; do echo "nginx auth-map $u/$u-pass-$4 -> $(code -u $u:$u-pass-$4 -H 'Host: map.example.com' 127.0.0.1:8880/)"; done
}
echo "== 1. public API user fields"
for m in default-digest@kubernetescrd default-basic@kubernetescrd default-ngfile-rule-0-path-0-basic-auth@kubernetesingressnginx default-ngmap-rule-0-path-0-basic-auth@kubernetesingressnginx; do echo "$m: $(users $m)"; done
echo "== 2. runtime authentication (version 1 credentials)"
auth digest-pass-1 basic-pass-1 file-pass-1 1
echo "wrong password: digest -> $(code --digest -u dana:nope 127.0.0.1:8880/digest/) basic -> $(code -u alice:nope 127.0.0.1:8880/basic/) file -> $(code -u nina:nope -H 'Host: file.example.com' 127.0.0.1:8880/) map -> $(code -u amy:nope -H 'Host: map.example.com' 127.0.0.1:8880/)"
echo "== 3. no-op Secret event (label only): emitted configuration count must not move"
echo "before: crd=$(emitted kubernetescrd) nginx=$(emitted kubernetesingressnginx)"
$K label secret basic-users digest-users nginx-map nginx-file audit-touch=$RANDOM --overwrite >/dev/null; sleep 5
echo "after:  crd=$(emitted kubernetescrd) nginx=$(emitted kubernetesingressnginx)"
echo "== 4. credential rotation under the same Secret key (auth-map recreated with keys in a different insertion order)"
$K create secret generic basic-users --from-literal=users="alice:{SHA}$(sha basic-pass-2)" --dry-run=client -o yaml | $K apply -f - >/dev/null
$K create secret generic digest-users --from-literal=users="dana:traefik:$(dg dana digest-pass-2)" --dry-run=client -o yaml | $K apply -f - >/dev/null
$K create secret generic nginx-file --from-literal=auth="	nina:{SHA}$(sha file-pass-2)  " --dry-run=client -o yaml | $K apply -f - >/dev/null
$K delete secret nginx-map >/dev/null; $K create secret generic nginx-map --from-literal=mia="{SHA}$(sha mia-pass-2)" --from-literal=zed="{SHA}$(sha zed-pass-2)" --from-literal=amy="{SHA}$(sha amy-pass-2)" >/dev/null; sleep 6
echo "after:  crd=$(emitted kubernetescrd) nginx=$(emitted kubernetesingressnginx)"
echo "-- old credentials"; auth digest-pass-1 basic-pass-1 file-pass-1 1
echo "-- new credentials"; auth digest-pass-2 basic-pass-2 file-pass-2 2
for m in default-digest@kubernetescrd default-basic@kubernetescrd default-ngfile-rule-0-path-0-basic-auth@kubernetesingressnginx default-ngmap-rule-0-path-0-basic-auth@kubernetesingressnginx; do echo "$m: $(users $m)"; done
echo "== 5. credential payloads in public surfaces (API rawdata, debug log)"
curl -s 127.0.0.1:8881/api/rawdata > /tmp/rawdata.json
for v in "$(sha basic-pass-1)" "$(sha basic-pass-2)" "$(dg dana digest-pass-1)" "$(dg dana digest-pass-2)" "$(sha file-pass-1)" "$(sha file-pass-2)" "$(sha amy-pass-1)" "$(sha zed-pass-2)"; do
  echo "payload#$((++i)): rawdata=$(grep -cF -- "$v" /tmp/rawdata.json) log=$(grep -cF -- "$v" "$LOG")"
done
echo "references in rawdata: $(grep -o 'urn:k8s:secret:[a-z-]*:[a-z]*' /tmp/rawdata.json | sort -u | tr '\n' ' ')"
