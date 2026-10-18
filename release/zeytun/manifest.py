#!/usr/bin/env python3
"""Create the release manifest that app maintainers pin verbatim."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parents[2]
dist = root / "dist"
tag = sys.argv[1]
commit = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
source = dist / "zeytun-core-source.tar.gz"
subprocess.run(["git", "archive", "--format=tar.gz", "--prefix=zeytun-core/", f"--output={source}", "HEAD"], check=True)


def asset(name):
    return {"name": name, "sha256": hashlib.sha256((dist / name).read_bytes()).hexdigest()}


manifest = {
    "schema": 1, "repository": "zeytun-labs/zeytun-core", "tag": tag, "commit": commit,
    "assets": {target: asset(f"zeytun-core-{target}.zip") for target in ("macos-aarch64", "windows-x86_64")},
    "source": asset(source.name),
}
(dist / "core-release.json").write_text(json.dumps(manifest, indent=2) + "\n")
(dist / "SHA256SUMS").write_text("".join(f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}\n" for path in sorted(dist.iterdir()) if path.is_file() and path.name != "SHA256SUMS"))
