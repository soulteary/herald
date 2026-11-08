"""Resolve the checked-out release source and refuse published versions.

Only the full OCI version is reserved; major/minor/latest remain floating aliases.
The checks are fail-closed observations, not an atomic registry lock against
other privileged writers. A partial publication must not be rebuilt in place.
"""
import argparse
import base64
import http.client
import json
import os
from pathlib import Path
import re
import subprocess
import urllib.error
import urllib.parse
import urllib.request


class GuardError(Exception):
    pass


SEMVER = re.compile(r"^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-((?:(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?$")


def version(tag):
    match = SEMVER.fullmatch(tag)
    if not match or len(tag[1:]) > 128 or any(int(n) > 9007199254740991 for n in match.groups()[:3]):
        raise GuardError("use v-prefixed stable or prerelease SemVer fitting an OCI tag; build metadata (+...) is unsupported")
    return {"tag": tag, "version": tag[1:], "stable": match.group(4) is None}


def git_value(args, cwd):
    try:
        result = subprocess.run(["git", *args], cwd=cwd, capture_output=True, text=True, timeout=15)
    except (OSError, subprocess.TimeoutExpired):
        raise GuardError("cannot observe local release source") from None
    if result.returncode:
        raise GuardError("release Git ref or tracked source is unproved")
    return result.stdout.strip()


def resolve(tag, cwd, expected_sha=None):
    result = version(tag)
    head = git_value(["rev-parse", "--verify", "HEAD^{commit}"], cwd)
    target = git_value(["rev-parse", "--verify", "refs/tags/" + tag + "^{commit}"], cwd)
    git_value(["diff", "--quiet", "HEAD", "--"], cwd)
    if not re.fullmatch(r"[0-9a-f]{40}", head) or head != target or (expected_sha and head != expected_sha):
        raise GuardError("checked-out HEAD does not match the release tag and expected revision")
    return {**result, "source_sha": head}


def event_tag(event, input_tag, ref):
    if event == "workflow_dispatch":
        return input_tag
    if event == "push" and ref.startswith("refs/tags/"):
        return ref[len("refs/tags/"):]
    raise GuardError("only a tag push or explicit existing-tag dispatch may release")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, file, code, message, headers, new_url):
        return None


class ReleaseHTTP:
    def __init__(self, repository, actor, token, api="https://api.github.com", registry="https://ghcr.io"):
        if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository) or not actor or not token or any(c in actor + token for c in "\r\n"):
            raise GuardError("release repository and service credentials are required")
        self.repository, self.actor, self.token = repository, actor, token
        self.api, self.registry = api.rstrip("/"), registry.rstrip("/")
        # The CLI uses fixed HTTPS origins. Constructor overrides exist only
        # for controlled local HTTP tests; environment variables cannot set them.
        self.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())

    def get(self, url, headers):
        request = urllib.request.Request(url, headers={"User-Agent": "herald-release-guard", **headers})
        try:
            try:
                response = self.opener.open(request, timeout=15)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                status = response.getcode()
                body = response.read(1048577)
        except (urllib.error.URLError, TimeoutError, OSError, http.client.HTTPException):
            raise GuardError("release service transport failed; absence is unproved") from None
        if len(body) > 1048576:
            raise GuardError("release service response exceeds its bound")
        return status, body

    @staticmethod
    def json_value(body):
        def invalid_constant(value):
            raise ValueError("non-JSON constant")

        def unique(pairs):
            result = {}
            for key, value in pairs:
                if key in result:
                    raise ValueError("duplicate JSON field")
                result[key] = value
            return result
        try:
            result = json.loads(body, object_pairs_hook=unique, parse_constant=invalid_constant)
        except (ValueError, UnicodeError):
            raise GuardError("release service returned malformed JSON") from None
        return result

    @classmethod
    def object(cls, body):
        result = cls.json_value(body)
        if type(result) is not dict:
            raise GuardError("release service returned an unexpected JSON shape")
        return result

    def require_absent(self, tag):
        release = version(tag)
        headers = {"Authorization": "Bearer " + self.token, "Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28"}
        status, body = self.get(self.api + "/repos/" + self.repository, headers)
        observed = self.object(body).get("full_name") if status == 200 else None
        if type(observed) is not str or observed.lower() != self.repository.lower():
            raise GuardError("repository access is unproved")
        status, body = self.get(self.api + "/repos/" + self.repository + "/releases/tags/" + urllib.parse.quote(tag, safe=""), headers)
        if status == 200:
            raise GuardError("release already exists; never delete or overwrite it")
        if status != 404 or self.object(body).get("message") != "Not Found":
            raise GuardError("release absence is unproved")
        # The tag endpoint finds published releases, not pending draft tags.
        # Authenticated push access can list drafts. Never infer absence from
        # an incomplete page walk or from an API failure.
        for page in range(1, 65):
            status, body = self.get(self.api + "/repos/" + self.repository + "/releases?per_page=100&page=" + str(page), headers)
            if status != 200:
                raise GuardError("draft release inventory is unproved")
            releases = self.json_value(body)
            if type(releases) is not list or len(releases) > 100:
                raise GuardError("draft release inventory is malformed")
            for item in releases:
                if type(item) is not dict or type(item.get("tag_name")) is not str or type(item.get("draft")) is not bool:
                    raise GuardError("draft release inventory is malformed")
                if item["tag_name"] == tag:
                    raise GuardError("published or draft release already exists; never overwrite it")
            if len(releases) < 100:
                break
        else:
            raise GuardError("release inventory exceeded its bound; absence is unproved")
        image = self.repository.lower()
        query = urllib.parse.urlencode({"service": "ghcr.io", "scope": "repository:" + image + ":pull"})
        basic = base64.b64encode((self.actor + ":" + self.token).encode()).decode()
        status, body = self.get(self.registry + "/token?" + query, {"Authorization": "Basic " + basic})
        if status != 200:
            raise GuardError("registry authorization failed; image absence is unproved")
        grant = self.object(body)
        token = grant.get("token", grant.get("access_token"))
        if type(token) is not str or not token or len(token) > 32768 or not re.fullmatch(r"[!-~]+", token):
            raise GuardError("registry token response is malformed")
        accept = ", ".join(("application/vnd.oci.image.index.v1+json", "application/vnd.docker.distribution.manifest.list.v2+json", "application/vnd.oci.image.manifest.v1+json", "application/vnd.docker.distribution.manifest.v2+json"))
        status, body = self.get(self.registry + "/v2/" + image + "/manifests/" + release["version"], {"Authorization": "Bearer " + token, "Accept": accept})
        if status == 200:
            raise GuardError("full-version image already exists; never rebuild or retag it")
        if status != 404:
            raise GuardError("image absence is unproved")
        errors = self.object(body).get("errors")
        if type(errors) is not list or not errors or any(type(e) is not dict or e.get("code") != "MANIFEST_UNKNOWN" for e in errors):
            raise GuardError("registry did not prove MANIFEST_UNKNOWN")
        return {"release_absent": True, "image_absent": True, "image_version": release["version"]}


def verify_metadata(image, image_version, tags):
    version("v" + image_version)
    expected = image.lower() + ":" + image_version
    if expected not in tags.splitlines():
        raise GuardError("metadata did not emit the guarded full-version image tag")


def main():
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest="command", required=True)
    source = sub.add_parser("resolve")
    source.add_argument("--event", required=True)
    source.add_argument("--input-tag", default="")
    source.add_argument("--ref", required=True)
    source.add_argument("--output", type=Path, required=True)
    guard = sub.add_parser("guard")
    guard.add_argument("--tag", required=True)
    guard.add_argument("--expected-sha", required=True)
    metadata = sub.add_parser("verify-metadata")
    metadata.add_argument("--image", required=True)
    metadata.add_argument("--version", required=True)
    args = parser.parse_args()
    try:
        if args.command == "resolve":
            result = resolve(event_tag(args.event, args.input_tag, args.ref), Path.cwd())
            with args.output.open("a") as output:
                for key, value in result.items():
                    output.write(key + "=" + (str(value).lower() if type(value) is bool else value) + "\n")
        elif args.command == "guard":
            result = resolve(args.tag, Path.cwd(), args.expected_sha)
            client = ReleaseHTTP(os.environ.get("GITHUB_REPOSITORY", ""), os.environ.get("GITHUB_ACTOR", ""), os.environ.get("GH_TOKEN", ""))
            result.update(client.require_absent(args.tag))
        else:
            verify_metadata(args.image, args.version, os.environ.get("METADATA_TAGS", ""))
            result = {"guarded_full_version_present": True}
        print(json.dumps(result, sort_keys=True))
    except GuardError as error:
        parser.exit(1, "release guard refused: " + str(error) + "\n")


if __name__ == "__main__":
    main()
