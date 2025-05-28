#!/usr/bin/env bash
# chaos_monkey.sh: randomly kills and restarts a platform service via docker-compose
# Requires docker-compose and services defined in docker-compose.yml

SERVICES=(api identities bookkeeper fiat marketfeeds trading)
# pick a random service
RAND=$((RANDOM % ${#SERVICES[@]}))
TARGET=${SERVICES[$RAND]}

echo "[ChaosMonkey] Targeting service: $TARGET"
# kill the service (stop container)
dockerCmd="docker-compose kill $TARGET"
eval $dockerCmd
echo "[ChaosMonkey] $TARGET killed"
sleep 15
# restart the service
upCmd="docker-compose up -d $TARGET"
eval $upCmd
echo "[ChaosMonkey] $TARGET restarted"
