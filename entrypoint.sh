#!/bin/sh
set -e

if [ -z "$1" ]; then
  echo "Usage: docker run --rm chunkcorp/simob <API_KEY>"
  echo "See https://simpleobservability.com/docs for help"
  exit 1
fi

/app/simob config "api_key=$1"
exec /app/simob start
