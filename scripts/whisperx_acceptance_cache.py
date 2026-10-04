"""Copy fixture cache bytes and internal links using Linux's filesystem view."""

import argparse
import os
import shutil
import stat
from pathlib import Path


def copy_cache(source, destination):
    source = source.resolve(strict=True)
    destination = destination.resolve(strict=True)
    if not source.is_dir() or not destination.is_dir():
        raise ValueError("cache source and destination must be directories")
    if destination == source or source in destination.parents or any(destination.iterdir()):
        raise ValueError("cache destination must be separate and empty")
    directories, files, links = [], [], []

    def unreadable(error):
        raise error

    for root, names, filenames in os.walk(source, followlinks=False, onerror=unreadable):
        for name in names + filenames:
            path = Path(root) / name
            relative = path.relative_to(source)
            mode = path.lstat().st_mode
            if stat.S_ISLNK(mode):
                target = path.resolve(strict=True).relative_to(source)
                links.append((relative, target, path.is_dir()))
            elif stat.S_ISDIR(mode):
                directories.append(relative)
            elif stat.S_ISREG(mode):
                files.append(relative)
            else:
                raise ValueError("cache contains a special file")
    # Do not copy mount-root ownership/mode/timestamps: Docker Desktop can
    # reject those operations. The harness separately provisions its new tree.
    for relative in directories:
        (destination / relative).mkdir(parents=True, exist_ok=True)
    for relative in files:
        shutil.copyfile(source / relative, destination / relative)
    for relative, target, directory in links:
        link = destination / relative
        link.symlink_to(
            os.path.relpath(destination / target, link.parent), target_is_directory=directory
        )


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path)
    parser.add_argument("destination", type=Path)
    args = parser.parse_args()
    copy_cache(args.source, args.destination)
