#!/usr/bin/env bash

set -o pipefail

client_id=$1
pem=$( cat $2 )

now=$(date +%s)
iat=$((${now} - 60)) # Issues 60 seconds in the past
exp=$((${now} + 600)) # Expires 10 minutes in the future

b64enc() { openssl base64 | tr -d '=' | tr '/+' '_-' | tr -d '\n'; }

header_json='{
    "typ":"JWT",
    "alg":"RS256"
}'
# Header encode
header=$( echo -n "${header_json}" | b64enc )

payload_json="{
    \"iat\":${iat},
    \"exp\":${exp},
    \"iss\":\"${client_id}\"
}"
# Payload encode
payload=$( echo -n "${payload_json}" | b64enc )

# Signature
header_payload="${header}"."${payload}"
signature=$(
    openssl dgst -sha256 -sign <(echo -n "${pem}") \
    <(echo -n "${header_payload}") | b64enc
)

# Create JWT
JWT="${header_payload}"."${signature}"
APP_TOKEN_URL=$( curl -s -H "Authorization: Bearer ${JWT}" -H "Accept: application/vnd.github.v3+json" https://api.github.com/app/installations | jq -r '.[0].access_tokens_url' )
export GITHUB_TOKEN=$( curl -s -X POST -H "Authorization: Bearer ${JWT}" -H "Accept: application/vnd.github.v3+json" ${APP_TOKEN_URL} | jq -r '.token' )

echo -n $GITHUB_TOKEN
