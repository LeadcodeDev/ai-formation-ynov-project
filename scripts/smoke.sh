#!/bin/sh
set -eu
api=${API_URL:-http://localhost:8080}
token() { make -s token USER="$1"; }
subject() {
  payload=$(echo "$1" | cut -d. -f2 | tr '_-' '/+')
  while [ $(( ${#payload} % 4 )) -ne 0 ]; do payload="$payload="; done
  echo "$payload" | base64 -d 2>/dev/null | sed -n 's/.*"sub":"\([^"]*\)".*/\1/p'
}
expect() {
  want=$1; shift
  got=$(curl -s -o /dev/null -w '%{http_code}' "$@")
  [ "$got" = "$want" ] || { echo "FAIL AC-12: want $want got $got: $*" >&2; exit 1; }
  echo "ok $want $*" | cut -c1-90
}
student=$(token student1); staff=$(token staff)
expect 200 "$api/healthz"
expect 401 "$api/loans/me"
expect 200 -H "Authorization: Bearer $student" "$api/loans/me"
expect 403 -H "Authorization: Bearer $student" "$api/loans/overdue"
expect 200 -H "Authorization: Bearer $staff" "$api/loans/overdue"
item=$(curl -s -H "Authorization: Bearer $student" "$api/items?available=true" | grep -o '"id":"[0-9a-f-]\{36\}","inventory_number"' | head -1 | sed 's/"id":"\([^"]*\)".*/\1/')
borrower=$(subject "$student")
expect 201 -X POST -H "Authorization: Bearer $staff" -d "{\"item_id\":\"$item\",\"borrower_sub\":\"$borrower\"}" "$api/loans"
expect 409 -X POST -H "Authorization: Bearer $staff" -d "{\"item_id\":\"$item\",\"borrower_sub\":\"$borrower\"}" "$api/loans"
echo "AC-12 smoke ok"
