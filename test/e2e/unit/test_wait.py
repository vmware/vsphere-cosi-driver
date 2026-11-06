"""Polling helper tests; need no cluster."""
import pytest

import e2e_wait


def test_returns_when_check_passes():
    calls = iter([(False, "not yet"), (True, "")])
    e2e_wait.until(lambda: next(calls), "thing", timeout=5, interval=0)


def test_timeout_names_what_and_last_detail():
    with pytest.raises(TimeoutError, match="thing.*still waiting"):
        e2e_wait.until(lambda: (False, "still waiting"), "thing", timeout=0, interval=0)


def test_check_error_is_not_retried():
    def boom():
        raise RuntimeError("real error")

    with pytest.raises(RuntimeError):
        e2e_wait.until(boom, "thing", timeout=5, interval=0)
