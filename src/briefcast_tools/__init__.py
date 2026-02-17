"""Python tooling helpers for Briefcast."""

from briefcast_tools.logging_config import log_extra, redact_sensitive, setup_logging

__all__ = ["greet", "log_extra", "redact_sensitive", "setup_logging"]


def greet(name: str) -> str:
    """Return a deterministic greeting used by smoke tests."""
    return f"hello, {name}"
