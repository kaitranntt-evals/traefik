#!/bin/bash
cd ~/repos/traefik
LOG=verify/logs; mkdir -p $LOG
echo "### go test (required)"; go test -count=1 -timeout=5m ./pkg/config/dynamic/... ./pkg/server/middleware/... ./pkg/middlewares/auth/... ./pkg/provider/kubernetes/crd/... ./pkg/provider/kubernetes/ingress-nginx/... > $LOG/required_go_test.log 2>&1; echo "exit=$?"
echo "### go vet (required)"; go vet ./pkg/config/dynamic/... ./pkg/server/middleware/... ./pkg/middlewares/auth/... ./pkg/provider/kubernetes/crd/... ./pkg/provider/kubernetes/ingress-nginx/... > $LOG/required_go_vet.log 2>&1; echo "exit=$?"
F=~/eval_tests/tests
cp $F/eval_crd_behavior_test.go $F/export_eval_test.go $F/eval_provider_to_builder_runtime_test.go pkg/provider/kubernetes/crd/
cp $F/eval_ingress_nginx_test.go $F/export_eval_ingress_nginx_test.go pkg/provider/kubernetes/ingress-nginx/
cp $F/eval_auth_sanitation_test.go pkg/middlewares/auth/
echo "### fixtures"
go test -v -race -count=1 -timeout=5m -run '^TestEval_' ./pkg/provider/kubernetes/crd ./pkg/provider/kubernetes/ingress-nginx ./pkg/middlewares/auth > $LOG/fixtures_main.log 2>&1; echo "exit=$?"
grep -E "^(--- |ok|FAIL|panic)" $LOG/fixtures_main.log
rm pkg/provider/kubernetes/crd/eval_crd_behavior_test.go pkg/provider/kubernetes/crd/export_eval_test.go pkg/provider/kubernetes/crd/eval_provider_to_builder_runtime_test.go pkg/provider/kubernetes/ingress-nginx/eval_ingress_nginx_test.go pkg/provider/kubernetes/ingress-nginx/export_eval_ingress_nginx_test.go pkg/middlewares/auth/eval_auth_sanitation_test.go
git status --short
