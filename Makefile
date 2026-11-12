.PHONY: test-gate test-gate-self update-ui-lint-baseline

# 95-128: scripts/test_gate.sh 의 go test -timeout 15m (패키지마다 적용)
test-gate:
	./scripts/test_gate.sh

test-gate-self:
	./scripts/test_gate_selftest.sh

update-ui-lint-baseline:
	./scripts/update_ui_lint_baseline.sh
