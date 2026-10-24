#!/usr/bin/env python3
"""Prove a built server supports default local auth without Clerk configuration.

Usage: python3 scripts/smoke-self-hosted.py /absolute/path/to/server
Uses a temporary database, loopback ports, and an explicit clean environment.
"""
import base64
import http.cookiejar
import pathlib
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request


def free_port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


def check(condition, message):
    if not condition:
        raise AssertionError(message)


def run(binary):
    port, render_port = free_port(), free_port()
    while render_port == port:
        render_port = free_port()
    origin = f'http://127.0.0.1:{port}'
    jar = http.cookiejar.CookieJar()
    client = urllib.request.build_opener(
        urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(jar))

    def request(path, fields=None, headers=None):
        data = urllib.parse.urlencode(fields).encode() if fields is not None else None
        req = urllib.request.Request(origin + path, data=data, headers=headers or {})
        try:
            response = client.open(req, timeout=5)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            return response.status, response.read().decode()

    def no_clerk(html):
        for marker in ('clerk-publishable-key', 'clerk-frontend-api', '/clerk.js', 'data-controller="clerk"'):
            check(marker not in html, f'Unexpected Clerk bootstrap: {marker}')

    with tempfile.TemporaryDirectory(prefix='exhibit-self-hosted-') as directory:
        # Deliberately do not inherit the caller's environment or load .env files.
        env = {
            'APP_ORIGIN': origin, 'RENDER_ORIGIN': f'http://127.0.0.1:{render_port}',
            'ADDR': f'127.0.0.1:{port}', 'RENDER_ADDR': f'127.0.0.1:{render_port}',
            'DATA_DIR': directory, 'PI_BIN': '/nonexistent',
        }
        # Match Compose's implicit service credential. Hosted startup must refuse
        # it before listening, even when the value came from the environment.
        hosted_env = dict(env, DATA_DIR=directory + '/hosted',
                          RENDER_ORIGIN=f'http://render.test:{render_port}',
                          CLERK_PUBLISHABLE_KEY='pk_test_' + base64.b64encode(b'example.clerk.accounts.dev$').decode().rstrip('='),
                          CLERK_SECRET_KEY='sk_test_example', AUTH_TOKEN='dev-token')
        refused = subprocess.run([binary], env=hosted_env, cwd=directory,
                                 stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=20)
        check(refused.returncode != 0 and b'hosted login refuses the default dev-token' in refused.stdout,
              'Hosted startup accepted the Compose default service token')
        with tempfile.TemporaryFile() as log:
            process = subprocess.Popen([binary], env=env, cwd=directory, stdout=log, stderr=log)
            try:
                deadline = time.monotonic() + 20
                while True:
                    check(process.poll() is None, 'Server exited during startup')
                    try:
                        status, html = request('/auth/login')
                        break
                    except (urllib.error.URLError, TimeoutError):
                        check(time.monotonic() < deadline, 'Server startup timed out')
                        time.sleep(0.1)
                check(status == 200 and 'action="/auth/local"' in html, 'Default local login missing')
                no_clerk(html)
                check(request('/api/artifacts')[0] == 401, 'Anonymous API was accessible')
                check(request('/auth/local', {'username': 'admin', 'password': 'incorrect'})[0] == 401,
                      'Wrong password accepted')
                status, html = request('/auth/local', {'username': 'admin', 'password': 'changeme'})
                check(status == 200 and 'action="/auth/local"' not in html, 'Local login failed')
                no_clerk(html)
                check(len(jar) > 0, 'No local session issued')
                check(request('/api/artifacts')[0] == 200, 'Local session cannot access API')
                session = '; '.join(f'{cookie.name}={cookie.value}' for cookie in jar)
                request('/auth/logout')
                check(request('/api/artifacts', headers={'Cookie': session})[0] == 401,
                      'Logged-out session still accepted')
                print('PASS: hosted dev-token refusal; clean-environment startup; local login form; no Clerk bootstrap;')
                print('      anonymous/wrong-password rejection; local session/API access; logout revocation.')
            finally:
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()


if __name__ == '__main__':
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    run(str(pathlib.Path(sys.argv[1]).resolve(strict=True)))
