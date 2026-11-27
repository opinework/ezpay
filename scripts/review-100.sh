#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
go_bin="${GO_BIN:-go}"
rounds="${1:-100}"
if ! [[ "$rounds" =~ ^[1-9][0-9]*$ ]]; then
    echo "Round count must be a positive integer" >&2
    exit 2
fi
artifacts="${REVIEW_ARTIFACTS_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/ezpay-review.XXXXXX")}"
mkdir -p "$artifacts"
printf 'round\tarea\tseed\tcpu\tresult\tlog\n' > "$artifacts/rounds.tsv"
areas=(official-settlement official-protocol monitor upstream lifecycle notification order-input auth-config blockchain full-regression)
patterns=(
    'TestOfficial(Settlement|Receipt|Rollback)'
    'Test(AlipayOfficial|WechatOfficial|OfficialOrderModes|ReliabilityOfficial(Query|EndToEnd))'
    'TestMonitor'
    'Test(ReliabilityUpstream|LegacyPaymentProtocols)'
    'TestReliability(Terminal|Withdrawal|PaymentSchema|Migration)'
    'TestReliability(Notification|PublicTransport|QueryMerging)'
    'TestReliability(MerchantRequest|OrderInput|BoundedAmounts)'
    'Test(Reliability(AdminToken|SensitiveLog|Operational|RateLimit)|OfficialSettings|OfficialRouting)'
    'TestReliability(Blockchain|WalletAddress|ConcurrentAmount|TronPagination)'
    '.'
)
cpus=(1 2 4)
printf 'Artifacts: %s\n' "$artifacts"
"$go_bin" version > "$artifacts/toolchain.txt"
git diff --stat > "$artifacts/change-stat.txt"
git ls-files -z --cached --others --exclude-standard -- '*.go' '*.mod' '*.sum' '*.html' '*.js' '*.css' '*.sh' '*.yaml' '*.yml' 'Dockerfile' 'Makefile' 'pkg/debian/control' | sort -z | xargs -0 sha256sum > "$artifacts/source-sha256.txt"
for ((round=1; round<=rounds; round++)); do
    area_index=$(((round-1)%10))
    cpu="${cpus[$(((round-1)%3))]}"
    seed=$((2026112700+round))
    log="$artifacts/round-$(printf '%03d' "$round").log"
    if "$go_bin" test -race -count=1 -shuffle="$seed" -cpu="$cpu" -timeout=120s ./... -run "${patterns[$area_index]}" > "$log" 2>&1; then
        printf '%d\t%s\t%d\t%d\tPASS\t%s\n' "$round" "${areas[$area_index]}" "$seed" "$cpu" "$log" >> "$artifacts/rounds.tsv"
        printf '[%03d/%03d] %s PASS\n' "$round" "$rounds" "${areas[$area_index]}"
    else
        printf '%d\t%s\t%d\t%d\tFAIL\t%s\n' "$round" "${areas[$area_index]}" "$seed" "$cpu" "$log" >> "$artifacts/rounds.tsv"
        printf '[%03d/%03d] %s FAIL: %s\n' "$round" "$rounds" "${areas[$area_index]}" "$log" >&2
        tail -80 "$log" >&2
        exit 1
    fi
done
"$go_bin" vet ./... > "$artifacts/vet.log" 2>&1
"$go_bin" build -o "$artifacts/ezpay" . > "$artifacts/build.log" 2>&1
git diff --check > "$artifacts/diff-check.log" 2>&1
if [[ -n "${GOVULNCHECK_BIN:-}" ]]; then
    "$GOVULNCHECK_BIN" ./... > "$artifacts/govulncheck.log" 2>&1
fi
sha256sum --check --status "$artifacts/source-sha256.txt"
printf 'Completed %d rounds. Manifest: %s/rounds.tsv\n' "$rounds" "$artifacts"
