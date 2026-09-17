"""Pack a docker-load tarball (server-app:latest) without a Docker daemon."""
from __future__ import annotations

import gzip
import hashlib
import io
import json
import lzma
import os
import shutil
import tarfile
import tempfile
import time
from pathlib import Path

SKIP_APK = {".PKGINFO", ".SIGN.RSA", ".SIGN.RSA.rsa-pub"}


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def extract_apk(apk: Path, dest: Path) -> None:
    with gzip.open(apk, "rb") as gz:
        with tarfile.open(fileobj=gz, mode="r:") as tar:
            for m in tar.getmembers():
                name = m.name.lstrip("./")
                if not name or name in SKIP_APK or name.startswith(".SIGN."):
                    continue
                if m.issym() or m.islnk():
                    continue
                if m.isdir():
                    (dest / name).mkdir(parents=True, exist_ok=True)
                    continue
                src = tar.extractfile(m)
                if src is None:
                    continue
                out = dest / name
                out.parent.mkdir(parents=True, exist_ok=True)
                out.write_bytes(src.read())


def extract_deb_data(deb: Path, dest: Path) -> None:
    raw = deb.read_bytes()
    if not raw.startswith(b"!<arch>\n"):
        raise SystemExit(f"not an ar archive: {deb}")
    off = 8
    blob = None
    name = ""
    while off + 60 <= len(raw):
        hdr = raw[off : off + 60]
        name = hdr[0:16].decode("ascii").strip().rstrip("/")
        size = int(hdr[48:58].decode("ascii").strip())
        off += 60
        part = raw[off : off + size]
        off += size + (size % 2)
        if name.startswith("data.tar"):
            blob = part
            break
    if blob is None:
        raise SystemExit(f"no data.tar in {deb}")
    if name.endswith(".xz") or blob[:6] == b"\xfd7zXZ\x00":
        blob = lzma.decompress(blob)
    elif name.endswith(".gz") or blob[:2] == b"\x1f\x8b":
        blob = gzip.decompress(blob)
    links: list[tuple[str, str]] = []
    with tarfile.open(fileobj=io.BytesIO(blob), mode="r:") as tar:
        for m in tar.getmembers():
            rel = m.name.lstrip("./")
            if not rel:
                continue
            out = dest / rel
            if m.isdir():
                out.mkdir(parents=True, exist_ok=True)
                continue
            if m.issym() or m.islnk():
                links.append((rel, m.linkname))
                continue
            src = tar.extractfile(m)
            if src is None:
                continue
            out.parent.mkdir(parents=True, exist_ok=True)
            out.write_bytes(src.read())
    for rel, target in links:
        dest_path = dest / rel
        if dest_path.exists():
            continue
        tgt = Path(target)
        cand = (dest_path.parent / tgt).resolve() if not tgt.is_absolute() else dest / str(tgt).lstrip("/\\")
        try:
            cand.relative_to(dest.resolve())
        except ValueError:
            cand = dest / str(tgt).lstrip("/\\")
        if cand.is_file():
            dest_path.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(cand, dest_path)


def copy_tree(src: Path, dst: Path) -> None:
    if dst.exists():
        shutil.rmtree(dst)
    shutil.copytree(src, dst)


def add_dir(tar: tarfile.TarFile, arcname: str, mtime: int) -> None:
    info = tarfile.TarInfo(arcname.rstrip("/") + "/")
    info.type = tarfile.DIRTYPE
    info.mode = 0o755
    info.uid = info.gid = 0
    info.uname = info.gname = "root"
    info.mtime = mtime
    tar.addfile(info)


def add_file(tar: tarfile.TarFile, path: Path, arcname: str, mtime: int, mode: int) -> None:
    info = tarfile.TarInfo(arcname.replace("\\", "/"))
    info.size = path.stat().st_size
    info.mode = mode
    info.uid = info.gid = 0
    info.uname = info.gname = "root"
    info.mtime = mtime
    info.type = tarfile.REGTYPE
    with path.open("rb") as f:
        tar.addfile(info, f)


def file_mode(arc: str) -> int:
    arc = arc.replace("\\", "/").lstrip("/")
    if arc == "app/server" or arc.endswith("/server"):
        return 0o755
    top = arc.split("/", 1)[0]
    if top in ("lib", "lib64", "usr") and (
        "/ld-linux" in "/" + arc or arc.endswith(".so") or ".so." in arc.split("/")[-1]
    ):
        return 0o755
    return 0o644


def walk_rootfs(root: Path, tar: tarfile.TarFile, mtime: int) -> None:
    dirs = set()
    files: list[tuple[Path, str, int]] = []
    for dirpath, dirnames, filenames in os.walk(root):
        rel_dir = os.path.relpath(dirpath, root)
        if rel_dir == ".":
            arc_dir = ""
        else:
            arc_dir = rel_dir.replace("\\", "/")
            dirs.add(arc_dir)
        for name in dirnames:
            p = (arc_dir + "/" + name).lstrip("/")
            dirs.add(p)
        for name in filenames:
            full = Path(dirpath) / name
            arc = (arc_dir + "/" + name).lstrip("/").replace("\\", "/")
            files.append((full, arc, file_mode(arc)))
    for d in sorted(dirs, key=lambda s: (s.count("/"), s)):
        add_dir(tar, d, mtime)
    for full, arc, mode in sorted(files, key=lambda x: x[1]):
        add_file(tar, full, arc, mtime, mode)


def main() -> None:
    temp = Path(os.environ["TEMP"])
    repo = Path(r"c:\Users\테크센터\PROJECT\JOB_PLANNING")
    server = repo / "server"
    assets = temp / "jp-image-assets"
    root = temp / "jp-image-root"
    binary = root / "app" / "server"
    if not binary.is_file():
        raise SystemExit(f"missing linux binary: {binary}")

    copy_tree(server / "web", root / "app" / "web")
    copy_tree(server / "migrations", root / "app" / "migrations")
    copy_tree(server / "templates", root / "app" / "templates")
    (root / "app" / "data").mkdir(parents=True, exist_ok=True)
    (root / "tmp").mkdir(parents=True, exist_ok=True)

    for apk in assets.glob("*.apk"):
        extract_apk(apk, root)
    for deb in assets.glob("*.deb"):
        extract_deb_data(deb, root)

    cacert = assets / "cacert.pem"
    cert_dst = root / "etc" / "ssl" / "certs" / "ca-certificates.crt"
    cert_dst.parent.mkdir(parents=True, exist_ok=True)
    if cacert.is_file():
        shutil.copyfile(cacert, cert_dst)

    mtime = int(time.time())
    layer_path = temp / "jp-layer.tar"
    if layer_path.exists():
        layer_path.unlink()
    with tarfile.open(layer_path, "w") as layer:
        walk_rootfs(root, layer, mtime)

    diff_id = "sha256:" + sha256_file(layer_path)
    created = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(mtime))
    config = {
        "created": created,
        "architecture": "amd64",
        "os": "linux",
        "config": {
            "Env": [
                "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
                "TZ=Asia/Seoul",
                "PORT=8080",
                "DB_TYPE=sqlite",
                "DB_PATH=/app/data/app.db",
                "APP_ENV=production",
            ],
            "Cmd": ["/app/server"],
            "WorkingDir": "/app",
            "Volumes": {"/app/data": {}},
        },
        "rootfs": {"type": "layers", "diff_ids": [diff_id]},
        "history": [{"created": created, "created_by": "JOB_PLANNING pack_server_app_tar"}],
    }
    config_bytes = json.dumps(config, separators=(",", ":"), sort_keys=True).encode("utf-8")
    config_id = sha256_bytes(config_bytes)
    layer_id = sha256_file(layer_path)

    out_tar = repo / "deploy" / "server-app.tar"
    out_tar.parent.mkdir(parents=True, exist_ok=True)
    work = Path(tempfile.mkdtemp(prefix="jp-save-"))
    try:
        (work / f"{config_id}.json").write_bytes(config_bytes)
        layer_dir = work / layer_id
        layer_dir.mkdir()
        (layer_dir / "VERSION").write_text("1.0\n", encoding="ascii")
        shutil.copyfile(layer_path, layer_dir / "layer.tar")
        legacy = {
            "id": layer_id,
            "created": created,
            "parent": None,
            "container_config": {
                "Hostname": "",
                "Domainname": "",
                "User": "",
                "AttachStdin": False,
                "AttachStdout": False,
                "AttachStderr": False,
                "Tty": False,
                "OpenStdin": False,
                "StdinOnce": False,
                "Env": None,
                "Cmd": ["/bin/sh", "-c", "#(nop) JOB_PLANNING image"],
                "Image": "",
                "Volumes": None,
                "WorkingDir": "/app",
                "Entrypoint": None,
                "OnBuild": None,
                "Labels": None,
            },
        }
        (layer_dir / "json").write_text(json.dumps(legacy), encoding="utf-8")
        manifest = [
            {
                "Config": f"{config_id}.json",
                "RepoTags": ["server-app:latest"],
                "Layers": [f"{layer_id}/layer.tar"],
            }
        ]
        (work / "manifest.json").write_text(json.dumps(manifest), encoding="utf-8")
        (work / "repositories").write_text(
            json.dumps({"server-app": {"latest": layer_id}}), encoding="utf-8"
        )
        if out_tar.exists():
            out_tar.unlink()
        with tarfile.open(out_tar, "w") as image:
            for p in sorted(work.rglob("*")):
                if p.is_dir():
                    continue
                image.add(p, arcname=str(p.relative_to(work)).replace("\\", "/"))
    finally:
        shutil.rmtree(work, ignore_errors=True)

    print(f"wrote {out_tar} ({out_tar.stat().st_size} bytes) layer={layer_id[:12]} config={config_id[:12]}")


if __name__ == "__main__":
    main()
