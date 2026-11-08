import base64
from contextlib import contextmanager
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import textwrap
import threading
import unittest
import urllib.parse

from release_guard import GuardError, ReleaseHTTP, event_tag, resolve, verify_metadata, version


@contextmanager
def services():
    state = {
        "tag": "v1.5.1",
        "repo": (200, {"full_name": "Owner/Project"}),
        "release": (404, {"message": "Not Found"}),
        "releases": (200, []),
        "token": (200, {"token": "fixture-registry-token"}),
        "image": (404, {"errors": [{"code": "MANIFEST_UNKNOWN"}]}),
        "requests": [],
    }

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_GET(self):
            parsed = urllib.parse.urlsplit(self.path)
            routes = {"/api/repos/Owner/Project": "repo",
                      "/api/repos/Owner/Project/releases/tags/" + state["tag"]: "release",
                      "/api/repos/Owner/Project/releases": "releases",
                      "/registry/token": "token",
                      "/registry/v2/owner/project/manifests/" + state["tag"][1:]: "image"}
            role = routes.get(parsed.path)
            if role == "releases":
                page = urllib.parse.parse_qs(parsed.query).get("page", ["1"])[0]
                if page != "1":
                    role = "releases-" + page
            state["requests"].append({"method": self.command, "role": role, "path": self.path,
                                      "authorization": self.headers.get("Authorization")})
            status, payload = state.get(role, (404, {"message": "unknown fixture path"}))
            if payload is None:
                self.close_connection = True
                return
            raw = payload if type(payload) is bytes else json.dumps(payload).encode()
            self.send_response(status)
            if status == 302:
                self.send_header("Location", "/must-not-follow")
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    base = "http://127.0.0.1:" + str(server.server_port)
    client = ReleaseHTTP("Owner/Project", "fixture-actor", "fixture-key", base + "/api", base + "/registry")
    try:
        yield state, client
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


class HTTPGuardTest(unittest.TestCase):
    def test_prerelease_checks_exact_full_version_and_build_metadata_has_no_requests(self):
        with services() as (state, client):
            state["tag"] = "v1.5.1-rc.1"
            self.assertEqual("1.5.1-rc.1", client.require_absent(state["tag"])["image_version"])
            self.assertTrue(state["requests"][-1]["path"].endswith("/manifests/1.5.1-rc.1"))
            state["requests"].clear()
            with self.assertRaises(GuardError):
                client.require_absent("v1.5.1+build.1")
            self.assertEqual([], state["requests"])

    def test_only_explicit_authenticated_absence_allows_full_version(self):
        with services() as (state, client):
            self.assertEqual({"release_absent": True, "image_absent": True, "image_version": "1.5.1"}, client.require_absent("v1.5.1"))
            requests = state["requests"]
            self.assertEqual(["repo", "release", "releases", "token", "image"], [r["role"] for r in requests])
            self.assertEqual("Bearer fixture-key", requests[0]["authorization"])
            self.assertEqual("Basic " + base64.b64encode(b"fixture-actor:fixture-key").decode(), requests[3]["authorization"])
            self.assertIn("scope=repository%3Aowner%2Fproject%3Apull", requests[3]["path"])
            self.assertEqual("Bearer fixture-registry-token", requests[4]["authorization"])
            self.assertTrue(all(r["method"] == "GET" for r in requests))
            self.assertNotIn("/manifests/latest", requests[4]["path"])

    def test_existing_release_or_image_is_never_republished(self):
        for role in ("release", "image"):
            with self.subTest(role=role), services() as (state, client):
                state[role] = (200, {})
                with self.assertRaises(GuardError):
                    client.require_absent("v1.5.1")
                if role == "release":
                    self.assertNotIn("token", [r["role"] for r in state["requests"]])

    def test_auth_network_and_server_errors_never_mean_absence(self):
        with services() as (state, client):
            defaults = {key: state[key] for key in ("repo", "release", "releases", "token", "image")}
            for role in defaults:
                for status in (401, 403, 429, 500, 503):
                    with self.subTest(role=role, status=status):
                        state.update(defaults)
                        state[role] = (status, {"message": "Not Found", "errors": [{"code": "MANIFEST_UNKNOWN"}]})
                        with self.assertRaises(GuardError):
                            client.require_absent("v1.5.1")
            state.update(defaults)
            state["token"] = (0, None)
            with self.assertRaises(GuardError):
                client.require_absent("v1.5.1")

    def test_malformed_or_ambiguous_responses_are_refused(self):
        cases = [
            ("repo", 200, {"full_name": 7}),
            ("release", 404, b"not-json"),
            ("release", 404, {"message": "unknown authorization"}),
            ("releases", 200, {}),
            ("releases", 200, [{"tag_name": None, "draft": True}]),
            ("token", 200, b"<html>unexpected</html>"),
            ("token", 200, []),
            ("token", 200, {"token": None}),
            ("token", 200, {"token": "bad\nvalue"}),
            ("token", 200, b'{"token":"a","token":"b"}'),
            ("image", 404, {"errors": [{"code": "UNAUTHORIZED"}]}),
            ("image", 404, {"errors": [{"code": "MANIFEST_UNKNOWN"}, {"code": "UNAUTHORIZED"}]}),
            ("image", 404, {"message": "manifest unknown without error code"}),
            ("image", 404, b'{"errors":[{"code":"MANIFEST_UNKNOWN"}],"invalid":NaN}'),
        ]
        with services() as (state, client):
            defaults = {key: state[key] for key in ("repo", "release", "releases", "token", "image")}
            for role, status, payload in cases:
                with self.subTest(role=role, payload=type(payload).__name__):
                    state.update(defaults)
                    state[role] = (status, payload)
                    with self.assertRaises(GuardError):
                        client.require_absent("v1.5.1")

    def test_existing_draft_on_later_page_and_failed_pagination_block(self):
        with services() as (state, client):
            state["releases"] = (200, [{"tag_name": "v0.0." + str(i), "draft": False} for i in range(100)])
            state["releases-2"] = (200, [{"tag_name": "v1.5.1", "draft": True}])
            with self.assertRaises(GuardError):
                client.require_absent("v1.5.1")
            self.assertNotIn("token", [r["role"] for r in state["requests"]])
            state["releases-2"] = (503, {"message": "temporary failure"})
            with self.assertRaises(GuardError):
                client.require_absent("v1.5.1")
            state["releases-2"] = (200, [])
            self.assertTrue(client.require_absent("v1.5.1")["release_absent"])

    def test_redirects_are_not_followed_and_responses_are_bounded(self):
        with services() as (state, client):
            state["repo"] = (302, {})
            with self.assertRaises(GuardError):
                client.require_absent("v1.5.1")
            self.assertEqual(1, len(state["requests"]))
            state["repo"] = (200, b"x" * 1048577)
            with self.assertRaises(GuardError):
                client.require_absent("v1.5.1")


class VersionTest(unittest.TestCase):
    def test_stable_and_prerelease_image_names_match_metadata_patterns(self):
        self.assertEqual({"tag": "v1.5.1", "version": "1.5.1", "stable": True}, version("v1.5.1"))
        self.assertEqual({"tag": "v1.5.1-rc.1", "version": "1.5.1-rc.1", "stable": False}, version("v1.5.1-rc.1"))
        verify_metadata("ghcr.io/Owner/Project", "1.5.1", "ghcr.io/owner/project:1.5.1\nghcr.io/owner/project:1.5\nghcr.io/owner/project:1\nghcr.io/owner/project:latest")
        verify_metadata("ghcr.io/Owner/Project", "1.5.1-rc.1", "ghcr.io/owner/project:1.5.1-rc.1")
        with self.assertRaises(GuardError):
            verify_metadata("ghcr.io/owner/project", "1.5.1", "ghcr.io/owner/project:1.5\nghcr.io/owner/project:latest")
        for floating in ("1", "1.5", "latest"):
            with self.subTest(floating=floating), self.assertRaises(GuardError):
                verify_metadata("ghcr.io/owner/project", floating, "ghcr.io/owner/project:" + floating)

    def test_build_metadata_invalid_semver_and_unsupported_lengths_are_explicitly_rejected(self):
        for tag in ("v1.5.1+build.1", "v1.5.1-rc.1+build.2", "1.5.1", "v01.5.1", "v1.5.1-01", "v1.5.1-", "v9007199254740992.0.0", "v1.2.3-" + "x" * 128):
            with self.subTest(tag=tag), self.assertRaises(GuardError):
                version(tag)


class GitSourceTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name)
        self.env = {key: os.environ[key] for key in ("PATH", "HOME", "TMPDIR") if key in os.environ}
        self.env.update(GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull, GIT_TERMINAL_PROMPT="0")
        self.git("init", "-q")
        (self.repo / "source.txt").write_text("fixture source\n")
        self.git("add", "source.txt")
        self.git("commit", "-qm", "fixture source")
        self.head = self.git("rev-parse", "HEAD")
        self.git("tag", "-a", "v1.5.1", "-m", "unsigned local test tag")

    def git(self, *args):
        result = subprocess.run(["git", "-c", "user.name=Release Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", *args], cwd=self.repo, env=self.env, text=True, capture_output=True, timeout=10)
        self.assertEqual(0, result.returncode, result.stderr)
        return result.stdout.strip()

    def test_push_and_dispatch_resolve_the_actual_annotated_tag_commit(self):
        self.assertNotEqual(self.head, self.git("rev-parse", "refs/tags/v1.5.1"))
        for event, input_tag, ref in (("push", "", "refs/tags/v1.5.1"), ("workflow_dispatch", "v1.5.1", "refs/heads/main")):
            with self.subTest(event=event):
                tag = event_tag(event, input_tag, ref)
                self.assertEqual(self.head, resolve(tag, self.repo)["source_sha"])
                output = self.repo / (event + ".outputs")
                script = Path(__file__).with_name("release_guard.py").resolve()
                run = subprocess.run([sys.executable, "-I", "-B", str(script), "resolve", "--event", event, "--input-tag", input_tag, "--ref", ref, "--output", str(output)], cwd=self.repo, env={**self.env, "GITHUB_SHA": "0" * 40}, capture_output=True, text=True, timeout=10)
                self.assertEqual(0, run.returncode, run.stderr)
                self.assertEqual(self.head, json.loads(run.stdout)["source_sha"])
                self.assertIn("source_sha=" + self.head, output.read_text())

    def test_old_tag_uses_pinned_external_tools_without_changing_source(self):
        # The release predates the helper: only the workflow checkout owns it.
        self.assertFalse((self.repo / ".github/scripts/release_guard.py").exists())
        with tempfile.TemporaryDirectory() as directory:
            workspace = Path(directory)
            tools = workspace / "workflow-tools"
            tools.mkdir()
            script = tools / ".github/scripts/release_guard.py"
            script.parent.mkdir(parents=True)
            script.write_bytes(Path(__file__).with_name("release_guard.py").read_bytes())
            for args in (("init", "-q"), ("add", "."), ("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "workflow tools")):
                subprocess.run(["git", *args], cwd=tools, env=self.env, check=True, capture_output=True)
            sha = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=tools, env=self.env, text=True).strip()
            workflow = (Path(__file__).parents[1] / "workflows/release.yml").read_text()
            block = workflow.split("      - name: Verify workflow tools revision\n", 1)[1].split("        run: |\n", 1)[1].split("\n      - name:", 1)[0]
            verify = textwrap.dedent(block)
            for expected, success in ((sha, True), ("0" * 40, False)):
                result = subprocess.run(["/bin/bash", "-e", "-c", verify], cwd=self.repo, env={**self.env, "GITHUB_WORKSPACE": str(workspace), "WORKFLOW_SHA": expected}, capture_output=True)
                self.assertEqual(success, result.returncode == 0)
            for event, input_tag, ref in (("workflow_dispatch", "v1.5.1", "refs/heads/main"), ("push", "", "refs/tags/v1.5.1"), ("push", "", "refs/heads/main")):
                resolve_block = workflow.split("      - name: Resolve release ref\n", 1)[1].split("        run: |\n", 1)[1].split("\n      - name:", 1)[0]
                rendered = textwrap.dedent(resolve_block)
                result = subprocess.run(["/bin/bash", "-e", "-c", rendered], cwd=self.repo, env={**self.env, "GITHUB_WORKSPACE": str(workspace), "GITHUB_EVENT_NAME": event, "INPUT_RELEASE_TAG": input_tag, "GITHUB_REF": ref, "GITHUB_OUTPUT": str(workspace / "outputs")}, text=True, capture_output=True)
                if ref == "refs/heads/main" and event == "push":
                    self.assertNotEqual(0, result.returncode)
                else:
                    self.assertEqual(0, result.returncode, result.stderr)
                    self.assertEqual(self.head, json.loads(result.stdout)["source_sha"])
            # Import the external helper in an actual child process; fixture HTTP
            # origins are constructor-only and never production CLI options.
            wrapper = workspace / "check.py"
            wrapper.write_text("import importlib.util,json,sys\nfrom pathlib import Path\nspec=importlib.util.spec_from_file_location('external_guard',sys.argv[1])\nm=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)\nr=m.resolve('v1.5.1',Path.cwd(),sys.argv[2])\nc=m.ReleaseHTTP('Owner/Project','fixture-actor','fixture-key',sys.argv[3],sys.argv[4])\nprint(json.dumps(c.require_absent(r['tag'])))\n")
            with services() as (state, client):
                # Derive the loopback origins from fixture requests, not secrets.
                api = client.api
                registry = client.registry
                for expected, success in ((self.head, True), ("0" * 40, False)):
                    state["requests"].clear()
                    result = subprocess.run([sys.executable, "-I", "-B", str(wrapper), str(script), expected, api, registry], cwd=self.repo, env=self.env, text=True, capture_output=True)
                    self.assertEqual(success, result.returncode == 0, result.stderr)
                    self.assertEqual(5 if success else 0, len(state["requests"]))
            self.assertEqual(self.head, self.git("rev-parse", "HEAD"))
            self.assertFalse((self.repo / ".github/scripts/release_guard.py").exists())

    def test_mismatched_tag_expected_revision_dirty_source_and_non_tag_events_fail(self):
        with self.assertRaises(GuardError):
            resolve("v1.5.1", self.repo, "0" * 40)
        (self.repo / "source.txt").write_text("uncommitted change\n")
        with self.assertRaises(GuardError):
            resolve("v1.5.1", self.repo)
        self.git("add", "source.txt")
        self.git("commit", "-qm", "other source")
        with self.assertRaises(GuardError):
            resolve("v1.5.1", self.repo)
        with self.assertRaises(GuardError):
            event_tag("push", "", "refs/heads/main")
        with self.assertRaises(GuardError):
            resolve("v9.9.9", self.repo)


class ReleaseShellTest(unittest.TestCase):
    def test_workflow_checkout_and_build_directory_contract(self):
        workflow = (Path(__file__).parents[1] / "workflows/release.yml").read_text()
        release = workflow.split("  report_release:", 1)[0]
        self.assertIn("working-directory: release-source", release)
        self.assertIn("ref: ${{ github.workflow_sha }}", release)
        self.assertIn("format('refs/tags/{0}', inputs.release_tag)", release)
        self.assertEqual(2, release.count("persist-credentials: false"))
        self.assertEqual(3, release.count('python3 -I -B "$GITHUB_WORKSPACE/workflow-tools/.github/scripts/release_guard.py"'))
        for line in ("path: workflow-tools", "path: release-source", "go-version-file: 'release-source/go.mod'", "cache-dependency-path: 'release-source/go.sum'", "context: ./release-source", "file: ./release-source/docker/Dockerfile"):
            self.assertIn(line, release)
        self.assertNotIn("python3 -I -B .github/scripts/release_guard.py", release)
        self.assertLess(release.index("Verify workflow tools revision"), release.index("Resolve release ref"))

    def test_rendered_create_step_only_creates_and_preserves_literal_notes(self):
        workflow = Path(__file__).parents[1] / "workflows" / "release.yml"
        tail = workflow.read_text().split("      - name: Create Release\n", 1)[1]
        run = tail.split("        run: |\n", 1)[1].split("\n  report_release:", 1)[0]
        script = textwrap.dedent(run)
        for stable, tag in ((True, "v1.5.1"), (False, "v1.5.1-rc.1")):
            with self.subTest(tag=tag), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                executable = root / "bin"
                executable.mkdir()
                (root / "release-source" / "dist").mkdir(parents=True)
                for asset in ("herald-linux-amd64", "herald-linux-arm64", "checksums.txt"):
                    (root / "release-source" / "dist" / asset).write_bytes(b"synthetic asset\n")
                os.symlink("/bin/cat", executable / "cat")
                gh = executable / "gh"
                gh.write_text("#!" + sys.executable + "\nimport json,os,sys\nfrom pathlib import Path\nPath(os.environ['GH_CAPTURE']).write_text(json.dumps(sys.argv[1:]))\n")
                gh.chmod(0o700)
                values = {"github.repository": "Owner/Project", "steps.release.outputs.tag": tag,
                          "steps.version.outputs.tag": tag, "steps.version.outputs.version": tag[1:],
                          "env.REGISTRY": "ghcr.io", "env.IMAGE_NAME": "owner/project"}
                rendered = script
                for key, value in values.items():
                    rendered = rendered.replace("${{ " + key + " }}", value)
                self.assertNotIn("${{", rendered)
                env = {"PATH": str(executable), "RUNNER_TEMP": str(root), "GITHUB_WORKSPACE": str(root), "GITHUB_REPOSITORY": "Owner/Project",
                       "RELEASE_TAG": tag, "STABLE_RELEASE": str(stable).lower(), "GH_CAPTURE": str(root / "argv.json")}
                result = subprocess.run(["/bin/bash", "-c", rendered], cwd=root / "release-source", env=env, text=True, capture_output=True, timeout=10)
                self.assertEqual(0, result.returncode, result.stderr)
                self.assertEqual("", result.stderr)
                args = json.loads((root / "argv.json").read_text())
                self.assertEqual(["release", "create", tag], args[:3])
                self.assertIn("--verify-tag", args)
                self.assertEqual([str(root / "release-source" / "dist" / asset) for asset in ("herald-linux-amd64", "herald-linux-arm64", "checksums.txt")], args[3:6])
                self.assertNotIn("--clobber", args)
                self.assertNotIn("delete", args)
                self.assertEqual(not stable, "--prerelease" in args)
                notes = (root / "release-notes.md").read_text()
                self.assertIn("```bash", notes)
                self.assertIn("docker pull ghcr.io/owner/project:" + tag[1:], notes)


if __name__ == "__main__":
    unittest.main()
