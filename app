#!/bin/bash

echo "In '$0' (host:$(hostname)) ${K_JOB_INDEX:+(Index:$K_JOB_INDEX) }args: $*"
echo sleeping ${SLEEP:-1}
sleep ${SLEEP:-1}
env | sort | grep -v -e JOBCONTROLLER -e ^TEST -e KUBERNETES -e K_HEADERS

# Fail 1/3 of the time if this is run as a batch job
if [[ -n "$PASS" ]] && [[ -n "$K_JOB_NAME" ]] && (( $RANDOM % 3 == 0 )) ; then
  exit 1
fi
