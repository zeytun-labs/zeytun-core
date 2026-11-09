#!/usr/bin/env python3
"""Build and package the Zeytun desktop core from this exact source checkout."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import zipfile

ROOT = Path(__file__).resolve().parents[2]


def run(*args, **kwargs):
    return subprocess.check_output(args, cwd=ROOT, text=True, **kwargs).strip()


def build(target, tag):
    settings = {
        "macos-aarch64": ("darwin", "arm64", "1", "DEFAULT_BUILD_TAGS", "zeytun-core"),
        "windows-x86_64": ("windows", "amd64", "0", "DEFAULT_BUILD_TAGS_WINDOWS", "zeytun-core.exe"),
    }
    goos, arch, cgo, tags_file, executable = settings[target]
    tags = (ROOT / "release" / tags_file).read_text().strip() + ",with_grpc,with_v2ray_api"
    flags = (ROOT / "release/LDFLAGS").read_text().strip()
    commit = run("git", "rev-parse", "HEAD")
    stage = ROOT / "dist" / target
    stage.mkdir(parents=True)
    env = dict(os.environ, CGO_ENABLED=cgo, GOOS=goos, GOARCH=arch, GOTOOLCHAIN="local")
    subprocess.run([
        "go", "build", "-mod=readonly", "-trimpath", "-tags", tags,
        "-ldflags", f"-X github.com/sagernet/sing-box/constant.Version={tag} {flags} -s -w -buildid=",
        "-o", str(stage / executable), "./cmd/sing-box",
    ], cwd=ROOT, env=env, check=True)
    if goos == "windows":
        # Use the same pinned Cronet revision as go.mod and the upstream packager.
        revision = (ROOT / ".github/CRONET_GO_VERSION").read_text().strip()
        if revision[:12] not in (ROOT / "go.mod").read_text():
            raise ValueError("Cronet DLL revision does not match go.mod")
        checkout = Path(os.environ["RUNNER_TEMP"]) / "cronet-dll"
        run("git", "init", str(checkout))
        run("git", "-C", str(checkout), "remote", "add", "origin", "https://github.com/sagernet/cronet-go.git")
        run("git", "-C", str(checkout), "sparse-checkout", "set", "--no-cone", "/lib/windows_amd64/libcronet.dll")
        run("git", "-C", str(checkout), "fetch", "--depth=1", "--filter=blob:none", "origin", revision)
        run("git", "-C", str(checkout), "checkout", "FETCH_HEAD")
        shutil.copy2(checkout / "lib/windows_amd64/libcronet.dll", stage / "libcronet.dll")
    subprocess.run([str(stage / executable), "version"], cwd=stage, check=True)
    config = stage / "smoke.json"
    config.write_text('{"outbounds":[{"type":"direct","tag":"direct"}]}\n')
    subprocess.run([str(stage / executable), "check", "-c", str(config)], cwd=stage, check=True)
    config.unlink()
    shutil.copy2(ROOT / "LICENSE", stage / "LICENSE")
    shutil.copy2(ROOT / "NOTICE.md", stage / "NOTICE.md")
    (stage / "build-info.json").write_text(json.dumps({
        "repository": "zeytun-labs/zeytun-core", "commit": commit, "tag": tag,
        "target": target, "go": run("go", "version"), "tags": tags,
    }, indent=2) + "\n")
    output = ROOT / "dist" / f"zeytun-core-{target}.zip"
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for path in sorted(stage.iterdir()):
            archive.write(path, path.name)
    print(f"{hashlib.sha256(output.read_bytes()).hexdigest()}  {output.name}")


if __name__ == "__main__":
    build(*sys.argv[1:])
