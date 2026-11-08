"""Offline parser controls; no compilation, subprocess, sockets or messaging."""
import importlib.util
import json
from pathlib import Path
import unittest

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('pilot_rate_runner', HERE / 'test-pilot-rate-router.py')
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class CaseEvidenceTest(unittest.TestCase):
    def test_actual_four_logs_require_exact_full_test_path(self):
        rows = json.loads((HERE / 'testdata/pilot-rate-actual-pass.json').read_text())['cases']
        self.assertEqual({case[0] for case in runner.CASES}, set(rows))
        for name, _, expected in runner.CASES:
            with self.subTest(case=name):
                self.assertTrue(runner.case_passed(0, rows[name]['actual_Go_PASS_lines'], expected))

    def test_parent_only_never_satisfies_child(self):
        parent = 'TestPilotProductionRouterAccountAndReceiverLimitsRemain'
        log = '--- PASS: ' + parent + ' (0.00s)\nPASS\n'
        for name, _, expected in runner.CASES:
            if '/' in expected:
                with self.subTest(case=name):
                    self.assertFalse(runner.case_passed(0, log, expected))

    def test_wrong_child_does_not_satisfy_expected_child(self):
        parent = 'TestPilotProductionRouterAccountAndReceiverLimitsRemain'
        log = '--- PASS: ' + parent + ' (0.00s)\n    --- PASS: ' + parent + '/other (0.00s)\nPASS\n'
        self.assertFalse(runner.case_passed(0, log, parent + '/user'))

    def test_skip_fail_nonzero_missing_package_pass_and_duplicate_refuse(self):
        expected = runner.CASES[0][2]
        good = '--- PASS: ' + expected + ' (0.10s)\nPASS\n'
        for code, text in ((1, good), (0, good + '--- SKIP: helper (0.00s)\n'),
                           (0, good + '--- FAIL: helper (0.00s)\n'),
                           (0, good.replace('\nPASS\n', '\n')), (0, good + good)):
            with self.subTest(code=code, text=text):
                self.assertFalse(runner.case_passed(code, text, expected))


if __name__ == '__main__':
    unittest.main()
