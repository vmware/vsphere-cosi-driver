"""Fake IAM service for the kind testbed. It answers the four calls the operator makes.

It runs in the cluster from a ConfigMap, so it uses the standard library only.
"""
import re
import threading
import uuid
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs
from xml.sax.saxutils import escape

PORT = 8080

# The only caller the fake accepts. The suite stores this pair in the store's admin Secret.
ADMIN_ACCESS_KEY = "iam-admin-key"
ADMIN_SECRET_KEY = "iam-admin-secret"

# MinIO knows only its root key, so every key the fake issues is that pair. It must match
# MINIO_ROOT_USER and MINIO_ROOT_PASSWORD in hack/kind/minio.yaml.
ISSUED_ACCESS_KEY = "minioadmin"
ISSUED_SECRET_KEY = "minioadmin123"

_XMLNS = "https://iam.amazonaws.com/doc/2010-05-08/"
_CALLER = re.compile(r"Credential=([^/,\s]+)/")


class State:
    """What the fake remembers: users, their inline policies and their keys."""

    def __init__(self):
        self.lock = threading.Lock()
        self.policies = {}
        self.keys = {}


class _Error(Exception):
    def __init__(self, status, code, message):
        super().__init__(message)
        self.status, self.code = status, code


def _now():
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _response(action, result=""):
    return (f'<{action}Response xmlns="{_XMLNS}">{result}'
            f"<ResponseMetadata><RequestId>{uuid.uuid4()}</RequestId></ResponseMetadata>"
            f"</{action}Response>")


def _create_user(state, p):
    name = p["UserName"]
    if name in state.policies:
        raise _Error(409, "EntityAlreadyExists", f"user {name} exists")
    state.policies[name], state.keys[name] = {}, set()
    user = (f"<User><Path>/</Path><UserName>{escape(name)}</UserName><UserId>{uuid.uuid4().hex[:21].upper()}</UserId>"
            f"<Arn>arn:aws:iam::000000000000:user/{escape(name)}</Arn><CreateDate>{_now()}</CreateDate></User>")
    return _response("CreateUser", f"<CreateUserResult>{user}</CreateUserResult>")


def _require_user(state, p):
    name = p["UserName"]
    if name not in state.policies:
        raise _Error(404, "NoSuchEntity", f"user {name} not found")
    return name


def _put_user_policy(state, p):
    state.policies[_require_user(state, p)][p["PolicyName"]] = p["PolicyDocument"]
    return _response("PutUserPolicy")


def _create_access_key(state, p):
    name = _require_user(state, p)
    state.keys[name].add(ISSUED_ACCESS_KEY)
    key = (f"<AccessKey><UserName>{escape(name)}</UserName><AccessKeyId>{ISSUED_ACCESS_KEY}</AccessKeyId>"
           f"<Status>Active</Status><SecretAccessKey>{ISSUED_SECRET_KEY}</SecretAccessKey>"
           f"<CreateDate>{_now()}</CreateDate></AccessKey>")
    return _response("CreateAccessKey", f"<CreateAccessKeyResult>{key}</CreateAccessKeyResult>")


def _delete_access_key(state, p):
    name = _require_user(state, p)
    if p["AccessKeyId"] not in state.keys[name]:
        raise _Error(404, "NoSuchEntity", "key not found")
    state.keys[name].discard(p["AccessKeyId"])
    return _response("DeleteAccessKey")


_ACTIONS = {
    "CreateUser": _create_user,
    "PutUserPolicy": _put_user_policy,
    "CreateAccessKey": _create_access_key,
    "DeleteAccessKey": _delete_access_key,
}


class _Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        try:
            body = self.rfile.read(int(self.headers.get("Content-Length") or 0)).decode()
            params = {k: v[0] for k, v in parse_qs(body).items()}
            caller = _CALLER.search(self.headers.get("Authorization", ""))
            if not caller or caller.group(1) != ADMIN_ACCESS_KEY:
                raise _Error(403, "AccessDenied", "only the store's admin key may call IAM")
            handler = _ACTIONS.get(params.get("Action"))
            if handler is None:
                raise _Error(400, "InvalidAction", f"unsupported action {params.get('Action')!r}")
            with self.server.state.lock:
                self._reply(200, handler(self.server.state, params))
        except _Error as e:
            self._reply(e.status, f'<ErrorResponse xmlns="{_XMLNS}"><Error><Type>Sender</Type>'
                                  f"<Code>{e.code}</Code><Message>{escape(str(e))}</Message></Error>"
                                  f"<RequestId>{uuid.uuid4()}</RequestId></ErrorResponse>")
        except KeyError as e:
            self._reply(400, f'<ErrorResponse xmlns="{_XMLNS}"><Error><Type>Sender</Type>'
                             f"<Code>MissingParameter</Code><Message>{escape(str(e))}</Message></Error>"
                             f"<RequestId>{uuid.uuid4()}</RequestId></ErrorResponse>")

    def _reply(self, status, xml):
        data = xml.encode()
        self.send_response(status)
        self.send_header("Content-Type", "text/xml")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def make_server(host, port):
    server = ThreadingHTTPServer((host, port), _Handler)
    server.state = State()
    return server


if __name__ == "__main__":
    make_server("0.0.0.0", PORT).serve_forever()
