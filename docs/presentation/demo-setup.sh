#!/usr/bin/env bash
# Makes the two made-up projects the demo works on, as git repos with one commit each.
# It stops if the folder exists, so it never overwrites work. `demo.sh reset` restores them
# to this commit and expects them at ~/demo-projects.
#
#   ./demo-setup.sh [folder]      (default ~/demo-projects)
set -euo pipefail

root=${1:-$HOME/demo-projects}
if [ -e "$root" ]; then
	echo "$root exists: remove it first if you want to start over" >&2
	exit 1
fi

commit() { # DIR MESSAGE
	git -C "$1" init -q
	git -C "$1" add -A
	git -C "$1" -c user.name=demo -c user.email=demo@example.com commit -qm "$2"
}

mkdir -p "$root/shop-api/tests" "$root/shop-web/static"

cat >"$root/shop-api/app.py" <<'EOF'
"""A tiny orders API."""

ORDERS = [{"id": i, "total": 10 * i, "status": "paid" if i % 2 else "pending"} for i in range(1, 101)]


def list_orders():
    return ORDERS


def get_order(order_id):
    for order in ORDERS:
        if order["id"] == order_id:
            return order
    return None
EOF
cat >"$root/shop-api/tests/test_checkout.py" <<'EOF'
import random

from app import get_order


def test_checkout_total():
    # Flaky on purpose: fails about one run in four.
    order = get_order(random.choice([1, 2, 3, 4]))
    assert order["total"] != 40
EOF
cat >"$root/shop-api/README.md" <<'EOF'
# shop-api

Orders API for a made-up shop. `list_orders` returns every order.
EOF
printf '__pycache__/\n' >"$root/shop-api/.gitignore"
commit "$root/shop-api" "Start shop-api"

cat >"$root/shop-web/index.html" <<'EOF'
<!doctype html>
<html>
<head><link rel="stylesheet" href="static/style.css"><title>Shop</title></head>
<body><h1>Shop</h1><a href="settings.html">Settings</a></body>
</html>
EOF
cat >"$root/shop-web/settings.html" <<'EOF'
<!doctype html>
<html>
<head><link rel="stylesheet" href="static/style.css"><title>Settings</title></head>
<body><h1>Settings</h1><label>Email <input type="email"></label></body>
</html>
EOF
cat >"$root/shop-web/static/style.css" <<'EOF'
body { font-family: sans-serif; background: #fff; color: #222; }
a { color: #06c; }
EOF
cat >"$root/shop-web/README.md" <<'EOF'
# shop-web

Web front end for a made-up shop. It has no dark mode yet.
EOF
commit "$root/shop-web" "Start shop-web"

echo "made $root/shop-api and $root/shop-web"
