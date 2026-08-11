#!/bin/sh
set -e

echo ""
echo "=============================================================="
echo " SIMOB AGENT - DOCKER IMAGE - FOR TESTING ONLY"
echo ""
echo " This image is for testing and evaluation."
echo " Do not use this image to monitor a production server."
echo ""
echo " Containers share the host kernel."
echo " Some metrics show the host, not the container."
echo " Disk usage, processes, SMART, and logs are not available."
echo ""
echo " For production, install the agent directly on your server."
echo " See https://simpleobservability.com/docs"
echo "=============================================================="
echo ""

if [ -z "$1" ]; then
  echo "Usage: docker run --rm chunkcorp/simob <API_KEY>"
  echo "See https://simpleobservability.com/docs for help"
  exit 1
fi

/app/simob config "api_key=$1"
exec /app/simob start
