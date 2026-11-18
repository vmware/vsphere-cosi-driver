"""The fake IAM service answers the calls the operator makes. No cluster needed."""
import threading
import urllib.error
import urllib.parse
import urllib.request

import pytest

from standins.iam import fake


@pytest.fixture
def server():
    srv = fake.make_server("127.0.0.1", 0)
    thread = threading.Thread(target=srv.serve_forever, daemon=True)
    thread.start()
    yield srv
    srv.shutdown()
    srv.server_close()


def call(srv, action, caller=fake.ADMIN_ACCESS_KEY, **params):
    """Returns (status, body) of one signed-looking IAM request."""
    body = urllib.parse.urlencode({"Action": action, "Version": "2010-05-08", **params}).encode()
    req = urllib.request.Request(
        f"http://127.0.0.1:{srv.server_address[1]}/", data=body, method="POST",
        headers={"Authorization": f"AWS4-HMAC-SHA256 Credential={caller}/20260101/us-east-1/iam/aws4_request, "
                                  "SignedHeaders=host, Signature=00",
                 "Content-Type": "application/x-www-form-urlencoded"})
    try:
        with urllib.request.urlopen(req) as resp:
            return resp.status, resp.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


def test_create_user_succeeds_once(server):
    status, _ = call(server, "CreateUser", UserName="op")
    assert status == 200


def test_create_user_twice_conflicts(server):
    call(server, "CreateUser", UserName="op")
    status, body = call(server, "CreateUser", UserName="op")
    assert status == 409
    assert "<Code>EntityAlreadyExists</Code>" in body


def test_put_policy_for_unknown_user_is_not_found(server):
    status, body = call(server, "PutUserPolicy", UserName="nobody", PolicyName="p", PolicyDocument="{}")
    assert status == 404
    assert "<Code>NoSuchEntity</Code>" in body


def test_put_policy_replaces_the_policy_of_the_same_name(server):
    call(server, "CreateUser", UserName="op")
    call(server, "PutUserPolicy", UserName="op", PolicyName="p", PolicyDocument="one")
    status, _ = call(server, "PutUserPolicy", UserName="op", PolicyName="p", PolicyDocument="two")
    assert status == 200
    assert server.state.policies["op"] == {"p": "two"}


def test_create_access_key_returns_the_issued_pair(server):
    call(server, "CreateUser", UserName="op")
    status, body = call(server, "CreateAccessKey", UserName="op")
    assert status == 200
    assert f"<AccessKeyId>{fake.ISSUED_ACCESS_KEY}</AccessKeyId>" in body
    assert f"<SecretAccessKey>{fake.ISSUED_SECRET_KEY}</SecretAccessKey>" in body


def test_create_access_key_for_unknown_user_is_not_found(server):
    status, _ = call(server, "CreateAccessKey", UserName="nobody")
    assert status == 404


def test_delete_access_key_removes_it_once(server):
    call(server, "CreateUser", UserName="op")
    call(server, "CreateAccessKey", UserName="op")
    first, _ = call(server, "DeleteAccessKey", UserName="op", AccessKeyId=fake.ISSUED_ACCESS_KEY)
    second, _ = call(server, "DeleteAccessKey", UserName="op", AccessKeyId=fake.ISSUED_ACCESS_KEY)
    assert (first, second) == (200, 404)


def test_caller_other_than_the_admin_is_denied(server):
    status, body = call(server, "CreateUser", caller="someone-else", UserName="op")
    assert status == 403
    assert "<Code>AccessDenied</Code>" in body


def test_unsigned_request_is_denied(server):
    req = urllib.request.Request(f"http://127.0.0.1:{server.server_address[1]}/", data=b"Action=CreateUser",
                                 method="POST")
    with pytest.raises(urllib.error.HTTPError) as err:
        urllib.request.urlopen(req)
    assert err.value.code == 403


def test_unknown_action_is_rejected(server):
    status, body = call(server, "ListUsers")
    assert status == 400
    assert "<Code>InvalidAction</Code>" in body
