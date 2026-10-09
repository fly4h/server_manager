#!/usr/bin/env python3
"""Push the current git worktree to GitHub via the Git Data REST API.

Why this exists:
  On hosts where plain `git push` over HTTPS is throttled / interrupted (SNI
  interference, large packfile POST timeouts), the REST API still works because
  it only exchanges small JSON chunks. Files are inlined into a single
  create-tree call.

Usage:
  python tools/gh-push.py --token <PAT> --repo owner/name [--branch main]
                          [--host-ip 20.205.243.168] [--message "..."]
"""

import argparse
import http.client
import json
import os
import socket
import ssl
import subprocess
import sys
import time

API_HOST = "api.github.com"


class GitHub:
    def __init__(self, token, ip, timeout=60):
        self.token = token
        self.ip = ip
        self.timeout = timeout

    def request(self, method, path, body=None, retries=3):
        payload = json.dumps(body).encode("utf-8") if body is not None else None
        last_err = None
        for attempt in range(retries):
            try:
                ctx = ssl.create_default_context()
                ctx.check_hostname = False
                ctx.verify_mode = ssl.CERT_NONE
                sock = socket.create_connection((self.ip, 443), timeout=self.timeout)
                sock = ctx.wrap_socket(sock, server_hostname=API_HOST)
                conn = http.client.HTTPSConnection(API_HOST, context=ctx, timeout=self.timeout)
                conn.sock = sock
                headers = {
                    "Authorization": f"Bearer {self.token}",
                    "User-Agent": "gh-push-tool",
                    "Accept": "application/vnd.github+json",
                    "Host": API_HOST,
                }
                if payload is not None:
                    headers["Content-Type"] = "application/json"
                    headers["Content-Length"] = str(len(payload))
                conn.request(method, path, body=payload, headers=headers)
                resp = conn.getresponse()
                raw = resp.read().decode("utf-8", errors="replace")
                conn.close()
                try:
                    return resp.status, json.loads(raw)
                except json.JSONDecodeError:
                    return resp.status, raw
            except Exception as exc:  # noqa: BLE001 - network flakiness is expected here
                last_err = exc
                time.sleep(2 * (attempt + 1))
        raise SystemExit(f"request {method} {path} failed: {last_err}")


def git(*args):
    out = subprocess.run(["git", *args], capture_output=True, text=True, encoding="utf-8")
    if out.returncode != 0:
        raise SystemExit(f"git {' '.join(args)} failed: {out.stderr.strip()}")
    return out.stdout


def collect_files(root):
    """Read content from the git index, not the worktree.

    The index holds the normalized form (`.gitattributes: * text=auto eol=lf`),
    so on Windows this avoids uploading CRLF files into the repository.
    """
    files = []
    for rel in git("-C", root, "ls-files").splitlines():
        rel = rel.strip()
        if not rel:
            continue
        blob = subprocess.run(
            ["git", "-C", root, "cat-file", "blob", f":{rel}"],
            capture_output=True,
        ).stdout
        if b"\x00" in blob:
            print(f"  skip (binary): {rel}", file=sys.stderr)
            continue
        files.append((rel, blob.decode("utf-8", "replace")))
    return files


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--token", required=True)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--branch", default="main")
    ap.add_argument("--host-ip", default="20.205.243.168")
    ap.add_argument("--message", default="")
    ap.add_argument("--root", default=".")
    ap.add_argument("--parent", default=None, help="force a specific parent commit sha")
    ap.add_argument("--force", action="store_true", help="allow non-fast-forward ref update")
    args = ap.parse_args()

    root = os.path.abspath(args.root)
    gh = GitHub(args.token, args.host_ip)

    files = collect_files(root)
    total = sum(len(c.encode("utf-8")) for _, c in files)
    print(f"collected {len(files)} files, {total/1024:.1f} KiB")

    tree = [
        {"path": path, "mode": "100644", "type": "blob", "content": content}
        for path, content in files
    ]
    status, res = gh.request("POST", f"/repos/{args.repo}/git/trees", {"tree": tree})
    if status == 409:
        # A completely empty repository rejects create-tree. Seed it with one
        # file through the Contents API, then build the real tree on top of it.
        print("empty repo detected: seeding initial commit via Contents API")
        status, res = gh.request(
            "PUT",
            f"/repos/{args.repo}/contents/.gitkeep",
            {"message": "chore: initialize repository", "content": ""},
        )
        if status not in (200, 201):
            raise SystemExit(f"seed commit failed [{status}]: {str(res)[:400]}")
        status, res = gh.request("POST", f"/repos/{args.repo}/git/trees", {"tree": tree})
    if status != 201:
        raise SystemExit(f"create tree failed [{status}]: {str(res)[:400]}")
    tree_sha = res["sha"]
    print(f"tree created: {tree_sha}")

    message = args.message or "chore: upload project via Git Data API"
    parents = []
    if args.parent:
        parents = [args.parent]
    else:
        status, parent_res = gh.request("GET", f"/repos/{args.repo}/git/ref/heads/{args.branch}")
        if status == 200:
            parents = [parent_res["object"]["sha"]]
    commit_body = {"message": message, "tree": tree_sha}
    if parents:
        commit_body["parents"] = parents
    status, res = gh.request("POST", f"/repos/{args.repo}/git/commits", commit_body)
    if status != 201:
        raise SystemExit(f"create commit failed [{status}]: {str(res)[:400]}")
    commit_sha = res["sha"]
    print(f"commit created: {commit_sha}")

    ref = f"refs/heads/{args.branch}"
    status, res = gh.request("GET", f"/repos/{args.repo}/git/ref/heads/{args.branch}")
    if status == 200:
        status, res = gh.request(
            "PATCH",
            f"/repos/{args.repo}/git/refs/heads/{args.branch}",
            {"sha": commit_sha, "force": bool(args.force)},
        )
    else:
        status, res = gh.request(
            "POST", f"/repos/{args.repo}/git/refs", {"ref": ref, "sha": commit_sha}
        )
    if status not in (200, 201):
        raise SystemExit(f"update ref failed [{status}]: {str(res)[:400]}")
    print(f"ref updated: {ref} -> {commit_sha}")
    print(f"DONE https://github.com/{args.repo}/tree/{args.branch}")


if __name__ == "__main__":
    main()
