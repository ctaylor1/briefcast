"""Cache copies keep internal links and reject paths outside the copied tree."""

import importlib.util
import os
from pathlib import Path

import pytest

spec = importlib.util.spec_from_file_location(
    "whisperx_acceptance_cache",
    Path(__file__).resolve().parents[1] / "scripts/whisperx_acceptance_cache.py",
)
cache = importlib.util.module_from_spec(spec)
spec.loader.exec_module(cache)


def symlink_or_skip(link, target):
    try:
        link.symlink_to(target)
    except OSError:
        pytest.skip("creating symlinks is unavailable on this host")


def test_copy_keeps_bytes_without_mutating_source_and_rebases_absolute_links(tmp_path):
    source, destination = tmp_path / "source", tmp_path / "destination"
    (source / "blobs").mkdir(parents=True)
    (source / "snapshots").mkdir()
    destination.mkdir()
    blob = source / "blobs/model"
    blob.write_bytes(b"model weights")
    symlink_or_skip(source / "snapshots/model.bin", blob)
    cache.copy_cache(source, destination)
    blob.write_bytes(b"source subsequently changed")
    assert (destination / "snapshots/model.bin").is_symlink()
    assert (destination / "snapshots/model.bin").read_bytes() == b"model weights"
    assert (source / "snapshots/model.bin").read_bytes() == b"source subsequently changed"


def test_escaping_link_rejected_before_copying_files(tmp_path):
    source, destination = tmp_path / "source", tmp_path / "destination"
    source.mkdir()
    destination.mkdir()
    outside = tmp_path / "private"
    outside.write_bytes(b"must not be copied")
    (source / "ordinary").write_bytes(b"fixture")
    symlink_or_skip(source / "escape", outside)
    with pytest.raises(ValueError):
        cache.copy_cache(source, destination)
    assert not list(destination.iterdir())


def test_destination_cannot_overwrite_prior_cache(tmp_path):
    source, destination = tmp_path / "source", tmp_path / "destination"
    source.mkdir()
    destination.mkdir()
    previous = destination / "model"
    previous.write_bytes(b"prior cache")
    with pytest.raises(ValueError):
        cache.copy_cache(source, destination)
    assert previous.read_bytes() == b"prior cache"


@pytest.mark.skipif(not hasattr(os, "mkfifo"), reason="FIFO creation requires Linux")
def test_special_file_rejected_before_copy(tmp_path):
    source, destination = tmp_path / "source", tmp_path / "destination"
    source.mkdir()
    destination.mkdir()
    (source / "ordinary").write_bytes(b"fixture")
    os.mkfifo(source / "fifo")
    with pytest.raises(ValueError, match="special file"):
        cache.copy_cache(source, destination)
    assert not list(destination.iterdir())
