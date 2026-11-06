"""Polling helper. Tests wait with this, never with sleep."""
import time


def until(check, what, timeout=60, interval=2):
    """Call check() until it returns (True, _) or the timeout passes.

    check returns (done, detail); detail explains the last failure. Raises
    TimeoutError naming `what` and that detail. An exception from check is
    not retried: it is a real error, not "not yet".
    """
    deadline = time.monotonic() + timeout
    while True:
        done, detail = check()
        if done:
            return
        if time.monotonic() >= deadline:
            raise TimeoutError(f"timed out after {timeout}s waiting for {what}: {detail}")
        time.sleep(interval)
