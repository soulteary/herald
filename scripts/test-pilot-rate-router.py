#!/usr/bin/env python3
"""Fixed synthetic loopback router regression; no real provider or deployment."""
import argparse
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
CASES = (
    ('shared-source-five-per-minute', '^TestPilotProductionRouterSharedGatewayBucketAndBodyCannotBypass$', 'TestPilotProductionRouterSharedGatewayBucketAndBodyCannotBypass'),
    ('trusted-proxy-split-and-spoof', '^TestPilotProductionRouterTrustedProxySplitsClientsAndBoundsChain$', 'TestPilotProductionRouterTrustedProxySplitsClientsAndBoundsChain'),
    ('per-account', '^TestPilotProductionRouterAccountAndReceiverLimitsRemain$/^user$', 'TestPilotProductionRouterAccountAndReceiverLimitsRemain/user'),
    ('per-receiver', '^TestPilotProductionRouterAccountAndReceiverLimitsRemain$/^destination$', 'TestPilotProductionRouterAccountAndReceiverLimitsRemain/destination'),
)


def case_passed(returncode, text, expected_full_test_path):
    lines = text.splitlines()
    if returncode != 0 or "PASS" not in lines:
        return False
    if any(re.match(r"^\s*--- (SKIP|FAIL):", line) for line in lines):
        return False
    passed_names = [match.group(1) for line in lines
                    if (match := re.fullmatch(r"\s*--- PASS: (\S+) \([^)]+\)", line))]
    return passed_names.count(expected_full_test_path) == 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output-parent', type=Path, help='parent for a new temporary evidence directory')
    args = parser.parse_args()
    go = shutil.which('go')
    if go is None:
        parser.error('existing Go executable must be available on PATH')
    if args.output_parent is not None:
        args.output_parent.mkdir(parents=True, exist_ok=True)
    output = Path(tempfile.mkdtemp(prefix='pilot-rate-', dir=args.output_parent)).resolve()
    # Preserve explicit caller Go/cache/offline settings, never deployment env.
    allowed = ('PATH', 'HOME', 'TMPDIR', 'TEMP', 'TMP', 'GOROOT', 'GOMODCACHE', 'GOCACHE',
               'GOTOOLCHAIN', 'GOPROXY', 'GOSUMDB', 'GONOSUMDB', 'GONOPROXY', 'GOPRIVATE',
               'GOFLAGS', 'CGO_ENABLED')
    env = {key: os.environ[key] for key in allowed if key in os.environ}
    binary = output / 'herald-router.test'
    record = {'scope': 'synthetic production-router/miniredis/provider-HTTP only; not real team capacity',
              'go': go, 'build_tag': 'pilot_rate_integration', 'compiled': False, 'cases': [], 'passed': False}
    try:
        with (output / 'compile.log').open('xb') as log:
            subprocess.run([go, 'test', '-race', '-tags=pilot_rate_integration', '-c', '-o', str(binary), './internal/router'],
                           cwd=ROOT, env=env, stdout=log, stderr=subprocess.STDOUT, check=True, timeout=180)
        record['compiled'] = True
        for name, selection, expected_full_test_path in CASES:
            directory = output / name
            directory.mkdir(mode=0o700)
            # Native audit is process-global: one fresh process/spool per case.
            with (directory / 'test.log').open('xb') as log:
                result = subprocess.run([str(binary), '-test.run=' + selection, '-test.v', '-test.timeout=20s'],
                                        cwd=directory, env=env, stdout=log, stderr=subprocess.STDOUT, timeout=30)
            text = (directory / 'test.log').read_text()
            passed = case_passed(result.returncode, text, expected_full_test_path)
            record['cases'].append({'case': name, 'selection': selection, 'exit_code': result.returncode,
                                    'expected_full_test_path': expected_full_test_path,
                                    'passed_without_skip': passed})
        record['passed'] = len(record['cases']) == 4 and all(row['passed_without_skip'] for row in record['cases'])
    except (OSError, subprocess.SubprocessError) as error:
        record['failure'] = type(error).__name__
    finally:
        # Evidence artifacts exclude executable and synthetic audit data.
        (output / 'result.json').write_text(json.dumps(record, indent=2) + '\n')
        print(json.dumps({'evidence_directory': str(output), 'passed': record['passed']}))
    return 0 if record['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
