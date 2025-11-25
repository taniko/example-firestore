#!/bin/bash
set -e

EXPORT_DIR=/data/saved

cleanup() {
  echo "Received shutdown signal, exporting data..."
  firebase emulators:export "$EXPORT_DIR" --project=demo-local --force
  echo "Export completed"
  exit 0
}

trap cleanup SIGTERM SIGINT

if [ -d "$EXPORT_DIR" ]; then
  echo "Importing existing data..."
  firebase emulators:start --project=demo-local --import="$EXPORT_DIR" &
else
  echo "Starting fresh (no existing data)..."
  firebase emulators:start --project=demo-local &
fi

wait $!
