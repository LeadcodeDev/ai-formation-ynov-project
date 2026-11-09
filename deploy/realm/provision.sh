#!/bin/sh
set -eu

BASE="${FERRISKEY_URL:-http://ferriskey:3333}"
ADMIN_USER="${FERRISKEY_ADMIN_USER:-admin}"
ADMIN_PASSWORD="${FERRISKEY_ADMIN_PASSWORD:-admin}"
REALM="${REALM:-loans}"
CLIENT_ID="${CLIENT_ID:-loans-api}"
AUDIENCE="${OIDC_AUDIENCE:-loans-api}"
USER_PASSWORD="${TEST_USER_PASSWORD:-Loans-Test-2026!}"
SCOPE_NAME="${CLIENT_ID}-audience"
USERS="student1:student student2:student staff1:staff admin1:admin"

out=$(mktemp)
status=0

call() {
  method=$1
  path=$2
  shift 2
  if [ "$#" -gt 0 ]; then
    status=$(curl -s -o "$out" -w '%{http_code}' -X "$method" "$BASE$path" \
      -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d "$1")
  else
    status=$(curl -s -o "$out" -w '%{http_code}' -X "$method" "$BASE$path" \
      -H "Authorization: Bearer $TOKEN")
  fi
}

must() {
  case "$status" in
    2??) ;;
    *)
      echo "FAILED $1: HTTP $status $(cat "$out")" >&2
      exit 1
      ;;
  esac
}

fetch_token() {
  curl -s -X POST "$BASE/realms/master/protocol/openid-connect/token" \
    -d grant_type=password -d client_id=admin-cli \
    -d "username=$ADMIN_USER" -d "password=$ADMIN_PASSWORD" | jq -r '.access_token // empty'
}

TOKEN=""
i=0
while [ -z "$TOKEN" ]; do
  TOKEN=$(fetch_token 2>/dev/null || true)
  [ -n "$TOKEN" ] && break
  i=$((i + 1))
  if [ "$i" -gt 60 ]; then
    echo "FerrisKey not reachable at $BASE" >&2
    exit 1
  fi
  sleep 2
done
echo "admin token obtained"

call GET "/realms/$REALM"
if [ "$status" = 200 ]; then
  echo "realm $REALM exists"
else
  call POST /realms "{\"name\":\"$REALM\"}"
  must "create realm"
  echo "realm $REALM created"
fi

R="/realms/$REALM"

call GET "$R/clients"
must "list clients"
CLIENT_UUID=$(jq -r --arg c "$CLIENT_ID" '.data[] | select(.client_id == $c) | .id' "$out")
if [ -z "$CLIENT_UUID" ]; then
  call POST "$R/clients" "$(jq -n --arg c "$CLIENT_ID" '{client_id:$c,name:$c,protocol:"openid-connect",client_type:"public",public_client:true,direct_access_grants_enabled:true,service_account_enabled:false,enabled:true}')"
  must "create client"
  CLIENT_UUID=$(jq -r .id "$out")
  echo "client $CLIENT_ID created"
else
  echo "client $CLIENT_ID exists"
fi

call GET "$R/client-scopes"
must "list client scopes"
SCOPE_UUID=$(jq -r --arg n "$SCOPE_NAME" '.data[] | select(.name == $n) | .id' "$out")
if [ -z "$SCOPE_UUID" ]; then
  call POST "$R/client-scopes" "$(jq -n --arg n "$SCOPE_NAME" '{name:$n,protocol:"openid-connect",description:"adds the API audience to access tokens"}')"
  must "create audience scope"
  SCOPE_UUID=$(jq -r .id "$out")
  echo "scope $SCOPE_NAME created"
else
  echo "scope $SCOPE_NAME exists"
fi

call GET "$R/client-scopes/$SCOPE_UUID"
must "get audience scope"
HAS_MAPPER=$(jq -r '[.protocol_mappers // [] | .[] | select(.mapper_type == "oidc-audience-mapper")] | length' "$out")
if [ "$HAS_MAPPER" = 0 ]; then
  call POST "$R/client-scopes/$SCOPE_UUID/protocol-mappers" "$(jq -n --arg a "$AUDIENCE" '{name:"audience",mapper_type:"oidc-audience-mapper",config:{"included.client.audience":$a,"access.token.claim":"true","id.token.claim":"false"}}')"
  must "create audience mapper"
  echo "audience mapper created"
else
  echo "audience mapper exists"
fi

call GET "$R/clients/$CLIENT_UUID/client-scopes"
must "list client scopes of client"
LINKED=$(jq -r --arg n "$SCOPE_NAME" '[.[] | select(.name == $n)] | length' "$out")
if [ "$LINKED" = 0 ]; then
  call PUT "$R/clients/$CLIENT_UUID/default-client-scopes/$SCOPE_UUID"
  must "assign audience scope"
  echo "audience scope assigned"
else
  echo "audience scope already assigned"
fi

for role in student staff admin; do
  call GET "$R/roles"
  must "list roles"
  exists=$(jq -r --arg r "$role" '[.data[] | select(.name == $r)] | length' "$out")
  if [ "$exists" = 0 ]; then
    call POST "$R/roles" "$(jq -n --arg r "$role" '{name:$r,permissions:[]}')"
    must "create role $role"
    echo "role $role created"
  else
    echo "role $role exists"
  fi
done

for entry in $USERS; do
  name=${entry%%:*}
  role=${entry##*:}
  call GET "$R/users"
  must "list users"
  uid=$(jq -r --arg u "$name" '.data[] | select(.username == $u) | .id' "$out")
  if [ -z "$uid" ]; then
    call POST "$R/users" "$(jq -n --arg u "$name" '{username:$u,email:($u + "@example.com"),firstname:$u,lastname:"Test",email_verified:true}')"
    must "create user $name"
    uid=$(jq -r '.data.id' "$out")
    echo "user $name created"
  else
    echo "user $name exists"
  fi

  call PUT "$R/users/$uid/reset-password" "$(jq -n --arg p "$USER_PASSWORD" '{credential_type:"password",temporary:false,value:$p}')"
  must "set password of $name"

  call GET "$R/roles"
  must "list roles"
  rid=$(jq -r --arg r "$role" '.data[] | select(.name == $r) | .id' "$out")
  call GET "$R/users/$uid/roles"
  must "list roles of $name"
  has=$(jq -r --arg r "$role" '[.data[] | select(.name == $r)] | length' "$out")
  if [ "$has" = 0 ]; then
    call POST "$R/users/$uid/roles/$rid"
    must "assign role $role to $name"
    echo "role $role assigned to $name"
  fi
done

echo "provisioning done"
