from briefcast_tools import greet


def test_greet_returns_expected_value() -> None:
    assert greet("briefcast") == "hello, briefcast"
