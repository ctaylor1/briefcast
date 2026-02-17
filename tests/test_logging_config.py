import json
import logging
from contextlib import redirect_stderr
from io import StringIO

from briefcast_tools.logging_config import log_extra, redact_sensitive, setup_logging


def test_redact_sensitive_recurses_nested_structures() -> None:
    source = {
        "token": "abc123",
        "nested": {"password": "secret", "safe": "value"},
        "items": [{"api_key": "key-1"}, {"value": 7}],
    }

    redacted = redact_sensitive(source)
    assert isinstance(redacted, dict)
    assert redacted["token"] == "***REDACTED***"
    nested = redacted["nested"]
    assert isinstance(nested, dict)
    assert nested["password"] == "***REDACTED***"
    assert nested["safe"] == "value"


def test_setup_logging_json_format_redacts_context(monkeypatch) -> None:
    monkeypatch.setenv("LOG_LEVEL", "INFO")
    monkeypatch.setenv("LOG_FORMAT", "json")

    capture = StringIO()
    with redirect_stderr(capture):
        setup_logging(service_name="briefcast-test", force=True)
        logger = logging.getLogger(__name__)
        logger.info(
            "hello",
            extra=log_extra({"api_key": "value", "safe": "ok", "password": "nope"}),
        )

    lines = [line for line in capture.getvalue().splitlines() if line.strip()]
    assert lines
    payload = json.loads(lines[-1])
    assert payload["service"] == "briefcast-test"
    assert payload["message"] == "hello"
    context = payload["context"]
    assert isinstance(context, dict)
    assert context["api_key"] == "***REDACTED***"
    assert context["password"] == "***REDACTED***"
    assert context["safe"] == "ok"
